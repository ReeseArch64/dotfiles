"""The unified entry point reuses healthy services and never auto-confirms a trial."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

HERE = Path(__file__).resolve().parent


class SecureAccessTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        self.access = self.root / 'access'
        self.access.mkdir()
        self.firewall = self.root / 'etc/homeprod-firewall'
        self.firewall.mkdir(parents=True)
        self.script = self.access / 'configure.sh'
        self.script.write_text((HERE / 'configure.sh').read_text().replace(
            '/etc/homeprod-firewall', str(self.firewall)))
        for component in ('tunnel', 'firewall'):
            folder = self.root / component
            folder.mkdir()
            (folder / 'configure.sh').write_text(f'''#!/bin/bash
set -eu
echo '{component}' "$@" >> "$TEST_ROOT/calls"
[[ "$SCENARIO" != {component}-failure ]]
''')
        self.env = dict(os.environ, PATH=str(self.bin) + ':' + os.environ['PATH'],
                        TEST_ROOT=str(self.root), SCENARIO='fresh')
        self.fake('id', 'echo 0')
        self.fake('systemctl', '''case "${!#}" in
 cloudflared-homeprod.service) [[ "$SCENARIO" == healthy || "$SCENARIO" == tunnel-only || "$SCENARIO" == missing-table ]] ;;
 homeprod-firewall.service) [[ "$SCENARIO" == healthy || "$SCENARIO" == missing-table ]] ;;
 *) exit 90 ;;
esac''')
        self.fake('curl', 'exit 0')
        self.fake('nft', '[[ "$SCENARIO" != missing-table ]]')

    def fake(self, name, body):
        p = self.bin / name
        p.write_text('#!/bin/bash\nset -eu\n' + body + '\n')
        p.chmod(0o755)

    def run_script(self, scenario='fresh'):
        self.env['SCENARIO'] = scenario
        return subprocess.run(['bash', str(self.script)], env=self.env,
                              capture_output=True, text=True, timeout=5)

    def calls(self):
        path = self.root / 'calls'
        return path.read_text().splitlines() if path.exists() else []

    def test_fresh_install_runs_tunnel_then_firewall_trial(self):
        result = self.run_script()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.calls(), ['tunnel', 'firewall apply'])

    def test_existing_confirmed_installation_is_not_changed(self):
        (self.firewall / 'rules.nft').touch()
        result = self.run_script('healthy')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.calls(), [])
        self.assertIn('credencial preservada', result.stdout)
        self.assertIn('sem novo teste', result.stdout)

    def test_tunnel_only_applies_firewall_without_asking_for_token(self):
        result = self.run_script('tunnel-only')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.calls(), ['firewall apply'])

    def test_missing_runtime_table_requires_new_trial(self):
        (self.firewall / 'rules.nft').touch()
        result = self.run_script('missing-table')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.calls(), ['firewall apply'])

    def test_pending_trial_is_never_confirmed_or_modified(self):
        (self.firewall / 'pending').mkdir()
        result = self.run_script('healthy')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.calls(), [])
        self.assertIn('manage.sh confirm', result.stderr)
        self.assertIn('manage.sh rollback', result.stderr)

    def test_failed_tunnel_does_not_apply_firewall(self):
        result = self.run_script('tunnel-failure')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.calls(), ['tunnel'])

    def test_firewall_failure_is_propagated(self):
        result = self.run_script('firewall-failure')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.calls(), ['tunnel', 'firewall apply'])


if __name__ == '__main__':
    unittest.main()
