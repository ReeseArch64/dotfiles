import copy
import unittest

from compose import transform


class TunnelComposeTests(unittest.TestCase):
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


if __name__ == '__main__':
    unittest.main()
