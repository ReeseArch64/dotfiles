"""Run the installer against fake commands and a temporary filesystem only."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

HERE = Path(__file__).resolve().parent


class HomeProdTunnelTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        self.state = self.root / 'etc/homeprod-cloudflared'
        self.unit = self.root / 'etc/systemd/system/cloudflared-homeprod.service'
        self.unit.parent.mkdir(parents=True)
        (self.root / 'run/lock').mkdir(parents=True)
        self.script = self.root / 'configure.sh'
        source = (HERE / 'configure.sh').read_text()
        for original, replacement in [
            ('/etc/', str(self.root / 'etc') + '/'),
            ('/run/lock/', str(self.root / 'run/lock') + '/'),
            ('/usr/share/keyrings/', str(self.root / 'keyrings') + '/'),
            ('/usr/bin/cloudflared', str(self.bin / 'cloudflared')),
        ]:
            source = source.replace(original, replacement)
        self.script.write_text(source)
        shutil.copy(HERE / 'cloudflared-homeprod.service', self.root)
        self.env = dict(os.environ, PATH=str(self.bin) + ':' + os.environ['PATH'],
                        TEST_ROOT=str(self.root), SCENARIO='success')
        self.fake('id', 'echo 0')
        self.fake('sleep', ':')
        self.fake('docker', '''[[ "$SCENARIO" != docker-failure ]] || exit 1
if [[ "$SCENARIO" == docker-conflict ]]; then echo docker.io/wisdomsky/cloudflared-web:latest; fi''')
        self.fake('cloudflared', '''[[ "$SCENARIO" != old-version ]] || exit 0
echo --token-file''')
        self.fake('curl', '''case "$*" in
  *127.0.0.1:80/*) [[ "$SCENARIO" != origin-failure ]] ;;
  *127.0.0.1:49312/ready*) [[ "$SCENARIO" != not-ready ]] ;;
  *) echo 'Unexpected network request' >&2; exit 90 ;;
esac''')
        # Preserve real install behavior, stripping ownership for non-root tests.
        install = shutil.which('install')
        self.fake('install', f'''args=()
while (($#)); do
  case "$1" in -o|-g) shift 2 ;; *) args+=("$1"); shift ;; esac
done
exec {install} "${{args[@]}}"''')
        self.fake('apt-get', "echo 'Unexpected package installation' >&2; exit 91")
        self.fake('systemctl', '''printf '%s\n' "$*" >> "$TEST_ROOT/systemctl.log"
command=$1
last=${!#}
case "$command" in
  cat) [[ "$SCENARIO" == service-conflict ]] ;;
  is-active) [[ -e "$TEST_ROOT/active" ]] ;;
  is-enabled) [[ -e "$TEST_ROOT/enabled" ]] ;;
  enable) touch "$TEST_ROOT/enabled" ;;
  disable) rm -f "$TEST_ROOT/enabled" ;;
  stop) rm -f "$TEST_ROOT/active" ;;
  start) touch "$TEST_ROOT/active" ;;
  restart)
    [[ "$SCENARIO" != restart-failure ]] || exit 12
    touch "$TEST_ROOT/active" ;;
  daemon-reload) : ;;
  *) exit 92 ;;
esac''')

    def fake(self, name, body):
        path = self.bin / name
        path.write_text('#!/usr/bin/env bash\nset -eu\n' + body + '\n')
        path.chmod(0o755)

    def run_script(self, scenario='success', answer='CONFIRMO', token='test-token'):
        self.env['SCENARIO'] = scenario
        return subprocess.run(['bash', str(self.script)], env=self.env,
                              input=answer + '\n' + token + '\n', text=True,
                              capture_output=True, timeout=15)

    def test_success_and_private_credential(self):
        result = self.run_script()
        self.assertEqual(result.returncode, 0, result.stderr)
        token = self.state / 'tunnel-token'
        self.assertEqual(token.read_text(), 'test-token')
        self.assertEqual(token.stat().st_mode & 0o777, 0o600)
        self.assertEqual(self.state.stat().st_mode & 0o777, 0o700)
        self.assertNotIn('test-token', result.stdout + result.stderr + self.unit.read_text())
        self.assertIn('NÃO considere a VPS protegida ainda', result.stdout)
        self.assertTrue((self.root / 'enabled').exists())
        self.assertTrue((self.root / 'active').exists())
        self.assertFalse(list(self.state.glob('backup.*/new-token')))

    def test_install_from_official_repository(self):
        (self.bin / 'cloudflared').unlink()
        (self.root / 'keyrings').mkdir()
        (self.root / 'etc/apt/sources.list.d').mkdir(parents=True)
        self.fake('curl', '''case "$*" in
  *cloudflare-main.gpg*) printf 'mock repository key' > "${!#}" ;;
  *127.0.0.1:80/*|*127.0.0.1:49312/ready*) : ;;
  *) exit 90 ;;
esac''')
        self.fake('apt-get', '''printf '%s\\n' "$*" >> "$TEST_ROOT/apt.log"
if [[ "$*" == 'install --yes cloudflared' ]]; then
  printf '#!/bin/sh\\necho --token-file\\n' > "$TEST_ROOT/bin/cloudflared"
  chmod +x "$TEST_ROOT/bin/cloudflared"
fi''')
        result = self.run_script()
        self.assertEqual(result.returncode, 0, result.stderr)
        repo = (self.root / 'etc/apt/sources.list.d/cloudflared.list').read_text()
        self.assertIn('signed-by=', repo)
        self.assertIn('https://pkg.cloudflare.com/cloudflared any main', repo)
        self.assertEqual((self.root / 'keyrings/cloudflare-main.gpg').read_text(),
                         'mock repository key')
        self.assertIn('install --yes cloudflared', (self.root / 'apt.log').read_text())

    def test_package_failure_does_not_install_service_or_credential(self):
        (self.bin / 'cloudflared').unlink()
        result = self.run_script()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.unit.exists())
        self.assertFalse((self.state / 'tunnel-token').exists())
        self.assertNotIn('test-token', result.stdout + result.stderr)

    def test_cancel_before_changes(self):
        result = self.run_script(answer='NAO')
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.state.exists())
        self.assertFalse(self.unit.exists())

    def test_invalid_token_before_changes(self):
        for token in ('', 'bad token', '--token secret'):
            with self.subTest(token=token):
                result = self.run_script(token=token)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(self.state.exists())

    def test_preflight_failures_leave_service_untouched(self):
        for scenario in ('docker-conflict', 'docker-failure', 'service-conflict', 'origin-failure'):
            with self.subTest(scenario=scenario):
                result = self.run_script(scenario)
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertFalse(self.unit.exists())
                self.assertFalse(self.state.exists())

    def test_unsupported_version_does_not_install_unit_or_token(self):
        result = self.run_script('old-version')
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.unit.exists())
        self.assertFalse((self.state / 'tunnel-token').exists())

    def test_failed_first_install_rolls_back(self):
        for scenario in ('restart-failure', 'not-ready'):
            with self.subTest(scenario=scenario):
                result = self.run_script(scenario)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(self.unit.exists())
                self.assertFalse((self.state / 'tunnel-token').exists())
                self.assertFalse((self.root / 'enabled').exists())
                self.assertFalse((self.root / 'active').exists())
                self.assertIn('Restaurando', result.stderr)

    def test_failed_update_restores_previous_unit_token_and_state(self):
        old_unit = '# Managed by dotfiles: homeprod-tunnel\n# previous unit\n'
        self.unit.write_text(old_unit)
        self.state.mkdir()
        old_token = self.state / 'tunnel-token'
        old_token.write_text('old-private-token')
        old_token.chmod(0o600)
        (self.root / 'active').touch()
        (self.root / 'enabled').touch()
        result = self.run_script('not-ready')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.unit.read_text(), old_unit)
        self.assertEqual(old_token.read_text(), 'old-private-token')
        self.assertTrue((self.root / 'enabled').exists())
        self.assertTrue((self.root / 'active').exists())
        self.assertNotIn('old-private-token', result.stdout + result.stderr)

    def test_unmanaged_unit_is_not_overwritten(self):
        self.unit.write_text('# unrelated service\n')
        result = self.run_script()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.unit.read_text(), '# unrelated service\n')
        self.assertFalse(self.state.exists())

    def test_rejects_symlinked_state(self):
        target = self.root / 'other'
        target.mkdir()
        self.state.symlink_to(target)
        result = self.run_script()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(list(target.iterdir()), [])

    def test_unit_does_not_contain_token_or_listen_publicly(self):
        unit = (HERE / 'cloudflared-homeprod.service').read_text()
        self.assertIn('DynamicUser=yes', unit)
        self.assertIn('LoadCredential=tunnel-token:', unit)
        self.assertIn('--token-file %d/tunnel-token', unit)
        self.assertIn('--metrics 127.0.0.1:49312', unit)
        self.assertNotIn('--token ', unit)
        self.assertNotIn('Environment=', unit)


if __name__ == '__main__':
    unittest.main()
