"""Unit and lifecycle tests: fake system services, real isolated temporary files."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

from rules import discover, render

HERE = Path(__file__).resolve().parent
SOCKETS = '''LISTEN 0 4096 0.0.0.0:22 0.0.0.0:* users:(("sshd",pid=793,fd=3))
LISTEN 0 4096 [::]:22 [::]:* users:(("sshd",pid=793,fd=4))
LISTEN 0 4096 *:80 *:* users:(("casaos-gateway",pid=858,fd=3))'''


class DiscoveryTests(unittest.TestCase):
    def test_inventory_preserves_only_real_external_ssh_ports(self):
        sockets = SOCKETS + '\nLISTEN 0 128 0.0.0.0:2222 *:* users:(("sshd",pid=1,fd=1))'
        sockets += '\nLISTEN 0 128 127.0.0.1:2200 *:* users:(("sshd",pid=1,fd=2))'
        result = discover([{'dev': 'eth0'}, {'dev': 'eth1'}], [], sockets)
        self.assertEqual(result, {'interfaces': ['eth0', 'eth1'], 'ssh_ports': [22, 2222]})

    def test_includes_nondefault_public_ipv6_interface(self):
        addresses = [{'ifname': 'eth1', 'addr_info': [{'local': '2606:4700::1111'}]},
                     {'ifname': 'docker0', 'addr_info': [{'local': '172.17.0.1'}]}]
        self.assertEqual(discover([{'dev': 'eth0'}], addresses, SOCKETS)['interfaces'], ['eth0', 'eth1'])

    def test_rejects_missing_or_unsafe_inventory(self):
        for routes, sockets, session in [([], SOCKETS, ''), ([{'dev': 'eth0'}], '', ''),
                                         ([{'dev': 'eth0"; drop'}], SOCKETS, ''),
                                         ([{'dev': 'eth0'}], SOCKETS, '1.2.3.4 12345 5.6.7.8 2222')]:
            with self.subTest(routes=routes, session=session), self.assertRaises(ValueError):
                discover(routes, [], sockets, session)

    def test_rule_order_and_scope(self):
        rules = render({'interfaces': ['eth0'], 'ssh_ports': [22]})
        self.assertIn('hook prerouting priority -150', rules)
        self.assertIn('ct direction reply ct state established accept', rules)
        self.assertNotIn('ct state established,related accept', rules)
        self.assertIn('fib daddr type local tcp dport { 22 }', rules)
        self.assertIn('iifname { "eth0" } jump public_in', rules)
        self.assertNotIn('flush ruleset', rules)
        self.assertNotIn('hook output', rules)
        self.assertNotIn('dport 80 accept', rules)
        unit = (HERE / 'homeprod-firewall.service').read_text()
        self.assertNotIn('ExecStop=', unit)
        self.assertIn('Before=network-pre.target docker.service', unit)


class LifecycleTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        self.state = self.root / 'etc/homeprod-firewall'
        self.unit = self.root / 'etc/systemd/system/homeprod-firewall.service'
        self.unit.parent.mkdir(parents=True)
        (self.root / 'run/lock').mkdir(parents=True)
        self.script = self.root / 'configure.sh'
        source = (HERE / 'configure.sh').read_text().replace('/etc/', str(self.root / 'etc') + '/')
        source = source.replace('/run/dotfiles-homeprod-firewall.lock', str(self.root / 'run/firewall.lock'))
        self.script.write_text(source)
        for name in ('rules.py', 'homeprod-firewall.service'):
            shutil.copy(HERE / name, self.root)
        self.env = dict(os.environ, PATH=str(self.bin) + ':' + os.environ['PATH'],
                        TEST_ROOT=str(self.root), SCENARIO='success')
        self.env.pop('SSH_CONNECTION', None)
        self.fake('id', 'echo 0')
        self.fake('curl', '[[ "$SCENARIO" != tunnel-failure ]]')
        self.fake('ufw', '''if [[ "$SCENARIO" == ufw-conflict ]]; then echo 'Status: active'; else echo 'Status: inactive'; fi''')
        self.fake('ip', '''if [[ "$*" == '-j address show' ]]; then echo '[]'
elif [[ "$*" == '-j -4 route show default' ]]; then echo '[{"dev":"eth0"}]'
else echo '[]'; fi''')
        self.fake('ss', "printf '%s\\n' '" + SOCKETS + "'")
        install = shutil.which('install')
        self.fake('install', f'''args=()
while (($#)); do
 case "$1" in -o|-g) shift 2 ;; *) args+=("$1"); shift ;; esac
done
exec {install} "${{args[@]}}"''')
        self.fake('systemd-run', '''echo "$*" >> "$TEST_ROOT/commands"
[[ "$SCENARIO" != timer-failure ]] || exit 1
touch "$TEST_ROOT/timer" "$TEST_ROOT/rollback-unit"''')
        self.fake('systemctl', '''echo "systemctl $*" >> "$TEST_ROOT/commands"
cmd=$1
last=${!#}
case "$cmd" in
 show)
   if [[ -f "$TEST_ROOT/rollback-unit" ]]; then echo loaded; else echo not-found; fi ;;
 cat) [[ -f "$TEST_ROOT/etc/systemd/system/homeprod-firewall.service" ]] ;;
 is-active)
   case "$last" in
     cloudflared-homeprod.service) [[ "$SCENARIO" != tunnel-failure ]] ;;
     homeprod-firewall-rollback.timer) [[ -f "$TEST_ROOT/timer" ]] ;;
     homeprod-firewall.service) [[ -f "$TEST_ROOT/active" ]] ;;
     *) [[ "$SCENARIO" == conflict && "$last" == nftables.service ]] ;;
   esac ;;
 is-enabled) [[ "$last" == homeprod-firewall.service && -f "$TEST_ROOT/enabled" ]] ;;
 enable) touch "$TEST_ROOT/enabled" ;;
 disable) rm -f "$TEST_ROOT/enabled" ;;
 start) touch "$TEST_ROOT/active" ;;
 reload) [[ "$SCENARIO" != persist-failure ]] ;;
 stop)
   for arg in "$@"; do
     case "$arg" in
       homeprod-firewall-rollback.timer) rm -f "$TEST_ROOT/timer" ;;
       homeprod-firewall.service) rm -f "$TEST_ROOT/active" ;;
     esac
   done ;;
 daemon-reload|reset-failed|--no-pager) : ;;
 *) exit 91 ;;
esac''')
        self.fake('nft', '''echo "nft $*" >> "$TEST_ROOT/commands"
if [[ "$*" == 'list table inet homeprod_guard' || "$*" == '-s list table inet homeprod_guard' ]]; then
 [[ -f "$TEST_ROOT/runtime" ]] || exit 1
 cat "$TEST_ROOT/runtime"
elif [[ "$1" == --check ]]; then
 [[ "$SCENARIO" != invalid-rules ]] || exit 1
elif [[ "$1" == -f ]]; then
 file=$2
 if [[ "$file" == */restore.nft && "$SCENARIO" == restore-failure ]]; then exit 4; fi
 if [[ "$file" == */candidate.nft ]]; then
   [[ "$SCENARIO" != apply-failure ]] || exit 1
   printf 'table inet homeprod_guard { # candidate\\n}\\n' > "$TEST_ROOT/runtime"
 elif grep -q 'table inet homeprod_guard {' "$file"; then
   grep -v '^delete table ' "$file" > "$TEST_ROOT/runtime"
 else
   rm -f "$TEST_ROOT/runtime"
 fi
else exit 92; fi''')

    def fake(self, name, body):
        p = self.bin / name
        p.write_text('#!/usr/bin/env bash\nset -eu\n' + body + '\n')
        p.chmod(0o755)

    def run_script(self, action='apply', scenario='success', answer='CONFIRMO', installed=False):
        self.env['SCENARIO'] = scenario
        script = self.state / 'manage.sh' if installed else self.script
        return subprocess.run(['bash', str(script), action], input=answer + '\n',
                              env=self.env, text=True, capture_output=True, timeout=15)

    def apply(self):
        p = self.run_script()
        self.assertEqual(p.returncode, 0, p.stdout + p.stderr)

    def test_trial_arms_timer_before_apply_and_does_not_persist(self):
        self.apply()
        self.assertTrue((self.state / 'pending/ready').exists())
        self.assertTrue((self.root / 'timer').exists())
        self.assertTrue((self.root / 'runtime').exists())
        self.assertFalse(self.unit.exists())
        self.assertFalse((self.root / 'enabled').exists())
        log = (self.root / 'commands').read_text()
        self.assertLess(log.index('--on-active=10m'), log.index('nft -f '))

    def test_confirm_enables_persistence_and_cancels_timer(self):
        self.apply()
        p = self.run_script('confirm', installed=True)
        self.assertEqual(p.returncode, 0, p.stdout + p.stderr)
        self.assertFalse((self.state / 'pending').exists())
        self.assertTrue(self.unit.exists())
        self.assertTrue((self.state / 'rules.nft').exists())
        self.assertTrue((self.root / 'enabled').exists())
        self.assertFalse((self.root / 'timer').exists())
        self.assertIn('systemctl reload homeprod-firewall.service', (self.root / 'commands').read_text())

    def test_rollback_uses_stable_installed_script_and_removes_only_trial(self):
        self.apply()
        p = self.run_script('rollback', installed=True)
        self.assertEqual(p.returncode, 0, p.stderr)
        self.assertFalse((self.root / 'runtime').exists())
        self.assertFalse((self.state / 'pending').exists())
        self.assertFalse((self.root / 'timer').exists())

    def test_preflight_failures_never_apply_rules(self):
        for scenario in ('conflict', 'ufw-conflict', 'tunnel-failure', 'invalid-rules'):
            with self.subTest(scenario=scenario):
                p = self.run_script(scenario=scenario)
                self.assertNotEqual(p.returncode, 0)
                self.assertFalse((self.root / 'runtime').exists())
                self.assertFalse((self.state / 'pending').exists())

    def test_apply_or_timer_failure_restores_network(self):
        for scenario in ('timer-failure', 'apply-failure'):
            with self.subTest(scenario=scenario):
                p = self.run_script(scenario=scenario)
                self.assertNotEqual(p.returncode, 0)
                self.assertFalse((self.root / 'runtime').exists())
                self.assertFalse((self.state / 'pending').exists())
                self.assertFalse((self.root / 'timer').exists())

    def test_confirmation_failure_restores_first_install(self):
        self.apply()
        p = self.run_script('confirm', scenario='persist-failure', installed=True)
        self.assertNotEqual(p.returncode, 0)
        self.assertFalse(self.unit.exists())
        self.assertFalse((self.state / 'rules.nft').exists())
        self.assertFalse((self.root / 'runtime').exists())
        self.assertFalse((self.root / 'enabled').exists())

    def test_previous_runtime_and_persistence_restored_after_failed_update(self):
        self.apply()
        self.assertEqual(self.run_script('confirm').returncode, 0)
        old_rules = (self.state / 'rules.nft').read_text()
        old_runtime = 'table inet homeprod_guard { # old-runtime\n}\n'
        (self.root / 'runtime').write_text(old_runtime)
        self.apply()
        p = self.run_script('confirm', scenario='persist-failure')
        self.assertNotEqual(p.returncode, 0, p.stderr)
        self.assertEqual((self.state / 'rules.nft').read_text(), old_rules)
        self.assertEqual((self.root / 'runtime').read_text(), old_runtime)
        self.assertTrue((self.root / 'active').exists())
        self.assertTrue((self.root / 'enabled').exists())

    def test_cannot_confirm_after_timer_expired_or_overwrite_pending(self):
        self.apply()
        self.assertNotEqual(self.run_script().returncode, 0)
        (self.root / 'timer').unlink()
        self.assertNotEqual(self.run_script('confirm').returncode, 0)
        self.assertTrue((self.state / 'pending').exists())
        self.assertEqual(self.run_script('rollback').returncode, 0)

    def test_declining_confirmation_leaves_rollback_armed(self):
        self.apply()
        self.assertNotEqual(self.run_script('confirm', answer='NAO').returncode, 0)
        self.assertTrue((self.root / 'timer').exists())

    def test_failed_restore_keeps_snapshot_and_armed_timer(self):
        self.apply()
        p = self.run_script('rollback', scenario='restore-failure', installed=True)
        self.assertNotEqual(p.returncode, 0)
        self.assertTrue((self.state / 'pending/ready').exists())
        self.assertTrue((self.root / 'timer').exists())
        self.assertTrue((self.root / 'runtime').exists())

    def test_unmanaged_table_is_never_overwritten(self):
        (self.root / 'runtime').write_text('table inet homeprod_guard {}\n')
        p = self.run_script()
        self.assertNotEqual(p.returncode, 0)
        self.assertEqual((self.root / 'runtime').read_text(), 'table inet homeprod_guard {}\n')


if __name__ == '__main__':
    unittest.main()
