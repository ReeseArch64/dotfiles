#!/usr/bin/env python3
"""Discover public ingress/SSH sockets and render our isolated nftables table."""
import ipaddress
import json
import os
from pathlib import Path
import re
import subprocess
import sys

MARKER = '# Managed by dotfiles: homeprod-firewall'
TABLE = 'homeprod_guard'


def discover(routes, addresses, sockets, ssh_connection=''):
    interfaces = set()
    for route in routes:
        if route.get('dev'):
            interfaces.add(route['dev'])
        interfaces.update(hop['dev'] for hop in route.get('nexthops', []) if hop.get('dev'))
    for interface in addresses:
        if any(ipaddress.ip_address(addr['local']).is_global
               for addr in interface.get('addr_info', []) if addr.get('local')):
            interfaces.add(interface['ifname'])
    interfaces.discard('lo')
    if not interfaces or any(not re.fullmatch(r'[A-Za-z0-9_.:-]{1,15}', name) for name in interfaces):
        raise ValueError('Interfaces públicas/default ausentes ou não suportadas; revise o inventário')

    ports = set()
    for line in sockets.splitlines():
        fields = line.split()
        if len(fields) < 6 or '"sshd"' not in line:
            continue
        host, _, port = fields[3].rpartition(':')
        host = host.strip('[]').split('%', 1)[0]
        if host not in ('*', '') and ipaddress.ip_address(host).is_loopback:
            continue
        if port.isdigit() and 0 < int(port) < 65536:
            ports.add(int(port))
    if not ports:
        raise ValueError('Nenhum socket TCP externo do sshd detectado; não presumirei porta 22')
    if ssh_connection:
        fields = ssh_connection.split()
        if len(fields) != 4 or not fields[3].isdigit() or int(fields[3]) not in ports:
            raise ValueError('Porta da sessão SSH atual diverge dos sockets sshd detectados')
    return {'interfaces': sorted(interfaces), 'ssh_ports': sorted(ports)}


def render(inventory):
    interfaces = ', '.join(json.dumps(name) for name in inventory['interfaces'])
    ports = ', '.join(str(port) for port in inventory['ssh_ports'])
    return f'''{MARKER}
# Runs after conntrack (-200), before Docker DNAT (-100).
# No global flush, output policy, Docker chain or SSH configuration changes.
add table inet {TABLE}
flush table inet {TABLE}
table inet {TABLE} {{
    chain public_in {{
        ct state invalid counter drop
        ct state related accept
        ct direction reply ct state established accept
        fib daddr type local tcp dport {{ {ports} }} accept
        fib daddr type {{ local, broadcast }} ip protocol udp udp sport 67 udp dport 68 accept
        fib daddr type {{ local, multicast }} meta nfproto ipv6 udp sport 547 udp dport 546 accept
        ip protocol icmp icmp type {{ destination-unreachable, time-exceeded, parameter-problem }} accept
        icmpv6 type {{ destination-unreachable, packet-too-big, time-exceeded, parameter-problem }} accept
        ip6 hoplimit 255 icmpv6 type {{ nd-router-advert, nd-neighbor-solicit, nd-neighbor-advert }} accept
        ip6 hoplimit 1 ip6 saddr fe80::/10 icmpv6 type mld-listener-query accept
        counter drop
    }}
    chain ingress {{
        type filter hook prerouting priority -150; policy accept;
        iifname {{ {interfaces} }} jump public_in
    }}
}}
'''


def main():
    inventory_path, rules_path = map(Path, sys.argv[1:])
    routes = []
    for family in ('-4', '-6'):
        routes.extend(json.loads(subprocess.check_output(
            ['ip', '-j', family, 'route', 'show', 'default'], text=True)))
    addresses = json.loads(subprocess.check_output(['ip', '-j', 'address', 'show'], text=True))
    sockets = subprocess.check_output(['ss', '-H', '-lntp'], text=True)
    inventory = discover(routes, addresses, sockets, os.environ.get('SSH_CONNECTION', ''))
    inventory_path.write_text(json.dumps(inventory, indent=2) + '\n')
    rules_path.write_text(render(inventory))
    print('Interfaces protegidas:', ', '.join(inventory['interfaces']))
    print('Portas SSH preservadas:', ', '.join(map(str, inventory['ssh_ports'])))


if __name__ == '__main__':
    main()
