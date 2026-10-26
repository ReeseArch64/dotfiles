#!/usr/bin/env python3
"""Transform normalized CasaOS Compose without embedding tunnel credentials."""
import json
import sys


def image_repository(image):
    for registry in ('docker.io/', 'index.docker.io/'):
        if image.startswith(registry):
            image = image[len(registry):]
    return image.split('@', 1)[0].split(':', 1)[0]


def transform(compose, kind, token_file):
    repositories = ('vaultwarden/server',) if kind == 'vault' else ('cloudflare/cloudflared', 'wisdomsky/cloudflared-web')
    matches = [s for s in compose['services'].values() if image_repository(s.get('image', '')) in repositories]
    if len(matches) != 1:
        raise ValueError('Esperado exatamente um serviço: ' + ', '.join(repositories))
    service = matches[0]
    if kind == 'tunnel' and service.get('network_mode') == 'host':
        del service['network_mode']
    elif service.get('network_mode'):
        raise ValueError('network_mode personalizado não suportado')
    compose.setdefault('networks', {})['vault_tunnel'] = {'external': True, 'name': 'homelab-vault-tunnel'}
    networks = service.setdefault('networks', {})
    if isinstance(networks, list):
        networks = {name: None for name in networks}
        service['networks'] = networks
    networks['vault_tunnel'] = {'aliases': ['vaultwarden-origin']} if kind == 'vault' else None
    env = service.get('environment') or {}
    if isinstance(env, list):
        env = dict(item.split('=', 1) for item in env)
    service['environment'] = env
    if kind == 'vault':
        env.update(DOMAIN='https://vault.reesearch64.com', SIGNUPS_ALLOWED='false', SIGNUPS_DOMAINS_WHITELIST='')
        service['ports'] = [{'target': 80, 'published': '10380', 'host_ip': '127.0.0.1', 'protocol': 'tcp'}]
        compose.setdefault('x-casaos', {}).update(scheme='https', hostname='vault.reesearch64.com', port_map='443')
    else:
        for name in list(env):
            if name.startswith('TUNNEL_'):
                del env[name]
        service.pop('env_file', None)
        service['entrypoint'] = ['cloudflared']
        service['command'] = ['tunnel', '--no-autoupdate', 'run', '--token-file', '/run/secrets/tunnel-token']
        service['ports'] = []
        # Only the Vaultwarden network is needed by this dedicated connector.
        service['networks'] = {'vault_tunnel': None}
        service['volumes'] = [{'type': 'bind', 'source': token_file, 'target': '/run/secrets/tunnel-token', 'read_only': True}]
        service['user'] = '65532:65532'
        service['restart'] = 'unless-stopped'
        service.pop('healthcheck', None)
    return compose


if __name__ == '__main__':
    kind, source, destination, token_file = sys.argv[1:]
    with open(source) as stream:
        result = transform(json.load(stream), kind, token_file)
    with open(destination, 'w') as stream:
        json.dump(result, stream, indent=2)
        stream.write('\n')
