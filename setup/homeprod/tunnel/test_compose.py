import copy
import tempfile
import unittest

from compose import transform, verify_vault_data_mount


class TunnelComposeTests(unittest.TestCase):
    def test_casaos_bridge_migrates_without_changing_data(self):
        source = {'services': {'vault': {'image': 'vaultwarden/server:1.36.0',
                  'network_mode': 'bridge',
                  'volumes': [{'type': 'bind', 'source': '/DATA/AppData/vaultwarden/data', 'target': '/data'}]}}}
        original = copy.deepcopy(source['services']['vault']['volumes'])
        result = transform(source, 'vault', '/token')
        self.assertEqual(result['networks']['vault_tunnel'],
                         {'external': True, 'name': 'homeprod-vault-tunnel'})
        service = result['services']['vault']
        self.assertNotIn('network_mode', service)
        self.assertEqual(service['volumes'], original)
        self.assertEqual(service['networks']['vault_tunnel']['aliases'], ['vaultwarden-origin'])

    def test_vault_mount_matches_running_container(self):
        with tempfile.TemporaryDirectory() as data:
            source = {'services': {'vault': {'image': 'vaultwarden/server:latest',
                      'volumes': [{'type': 'bind', 'source': data, 'target': '/data'}]}}}
            verify_vault_data_mount(source, [{'Type': 'bind', 'Source': data, 'Destination': '/data'}])
            self.assertFalse(source['services']['vault']['volumes'][0]['bind']['create_host_path'])
            for mounts in [[], [{'Type': 'bind', 'Source': '/wrong/data', 'Destination': '/data'}],
                           [{'Type': 'volume', 'Source': data, 'Destination': '/data'}]]:
                with self.assertRaises(ValueError):
                    verify_vault_data_mount(source, mounts)

    def test_vault_preserves_data_and_restricts_http(self):
        source = {'services': {'vault': {'image': 'vaultwarden/server:1.36.0',
                  'volumes': [{'source': '/DATA/vault', 'target': '/data', 'type': 'bind'}],
                  'environment': ['ADMIN_TOKEN=existing', 'DOMAIN=https://old.example'],
                  'networks': ['existing']}}, 'x-casaos': {'title': {'en_us': 'Vault'}}}
        data = copy.deepcopy(source['services']['vault']['volumes'])
        result = transform(source, 'vault', '/secret')
        service = result['services']['vault']
        self.assertEqual(service['volumes'], data)
        self.assertEqual(service['environment']['ADMIN_TOKEN'], 'existing')
        self.assertEqual(service['environment']['DOMAIN'], 'https://vault.reesearch64.com')
        self.assertEqual(service['environment']['SIGNUPS_ALLOWED'], 'false')
        self.assertEqual(service['ports'][0]['host_ip'], '127.0.0.1')
        self.assertIn('existing', service['networks'])

    def test_connector_has_no_inline_credentials_or_listening_ports(self):
        source = {'services': {'tunnel': {'image': 'cloudflare/cloudflared:latest',
                  'command': ['tunnel', 'run', '--token', 'secret'],
                  'environment': {'TUNNEL_TOKEN': 'secret'}, 'env_file': ['old.env'],
                  'volumes': ['/var/run/docker.sock:/var/run/docker.sock'],
                  'networks': ['default'], 'ports': ['8080:8080']}}}
        service = transform(source, 'tunnel', '/etc/private/token')['services']['tunnel']
        self.assertNotIn('secret', service['command'])
        self.assertNotIn('TUNNEL_TOKEN', service['environment'])
        self.assertNotIn('env_file', service)
        self.assertEqual(service['ports'], [])
        self.assertEqual(list(service['networks']), ['vault_tunnel'])
        self.assertEqual(service['volumes'][0]['source'], '/etc/private/token')
        self.assertTrue(service['volumes'][0]['read_only'])

    def test_rejects_host_network_and_ambiguous_service(self):
        for services in [
            {'one': {'image': 'vaultwarden/server:latest', 'network_mode': 'host'}},
            {'one': {'image': 'vaultwarden/server:latest'}, 'two': {'image': 'vaultwarden/server:latest'}},
        ]:
            with self.assertRaises(ValueError):
                transform({'services': services}, 'vault', '/secret')

    def test_casaos_wrapper_migrates_host_network_without_replacing_image(self):
        for image in ('wisdomsky/cloudflared-web:2026.5.0',
                      'docker.io/wisdomsky/cloudflared-web:2026.5.0@sha256:abc',
                      'index.docker.io/cloudflare/cloudflared@sha256:abc'):
            with self.subTest(image=image):
                source = {'services': {'cloudflared': {
                    'image': image, 'network_mode': 'host',
                    'entrypoint': ['node', '/var/app/backend/app.js'],
                    'ports': ['14333:14333'], 'volumes': ['/DATA/config:/config'],
                }}, 'x-casaos': {'main': 'cloudflared'}}
                result = transform(source, 'tunnel', '/etc/private/token')
                service = result['services']['cloudflared']
                self.assertEqual(service['image'], image)
                self.assertNotIn('network_mode', service)
                self.assertEqual(service['entrypoint'], ['cloudflared'])
                self.assertEqual(service['ports'], [])
                self.assertEqual(service['networks'], {'vault_tunnel': None})
                self.assertEqual(result['x-casaos']['main'], 'cloudflared')

    def test_rejects_two_supported_connectors_or_unrelated_image(self):
        for images in [('cloudflare/cloudflared:latest', 'wisdomsky/cloudflared-web:latest'),
                       ('untrusted/cloudflared:latest',)]:
            with self.assertRaises(ValueError):
                transform({'services': {str(i): {'image': image} for i, image in enumerate(images)}},
                          'tunnel', '/secret')


if __name__ == '__main__':
    unittest.main()
