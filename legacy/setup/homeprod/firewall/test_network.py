"""Packet-level checks in disposable user/network namespaces, never on the host."""
import ctypes
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import time
import unittest

from rules import render

SERVER = r'''
import selectors, socket, sys
sel = selectors.DefaultSelector()
for address, port in __import__('json').loads(sys.argv[1]):
    family = socket.AF_INET6 if ':' in address else socket.AF_INET
    s = socket.socket(family)
    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    if family == socket.AF_INET6:
        s.setsockopt(socket.IPPROTO_IPV6, socket.IPV6_V6ONLY, 1)
    s.bind((address, port)); s.listen(); s.setblocking(False)
    sel.register(s, selectors.EVENT_READ, True)
print('ready', flush=True)
while True:
    for key, _ in sel.select():
        if key.data:
            c, _ = key.fileobj.accept(); c.setblocking(False)
            sel.register(c, selectors.EVENT_READ, False)
        else:
            data = key.fileobj.recv(64)
            if data: key.fileobj.sendall(data)
            else: sel.unregister(key.fileobj); key.fileobj.close()
'''


UDP_SERVER = r'''
import selectors, socket, sys, json
sel = selectors.DefaultSelector()
for address, port in json.loads(sys.argv[1]):
    s = socket.socket(socket.AF_INET6 if ':' in address else socket.AF_INET, socket.SOCK_DGRAM)
    if ':' in address: s.setsockopt(socket.IPPROTO_IPV6, socket.IPV6_V6ONLY, 1)
    s.bind((address, port)); sel.register(s, selectors.EVENT_READ)
print('ready', flush=True)
while True:
    for key, _ in sel.select():
        data, peer = key.fileobj.recvfrom(64)
        key.fileobj.sendto(data, peer)
'''


def packet_checks():
    # A fresh unshare -Urn namespace has only loopback. Refuse any other context.
    assert os.environ.get('HOMEPROD_TEST_NETNS') == '1'
    assert len(json.loads(subprocess.check_output(['ip', '-j', 'link']))) == 1
    processes = []
    sockets = []
    libc = ctypes.CDLL(None, use_errno=True)

    def run(args, pid=None, **kwargs):
        prefix = ['nsenter', '-t', str(pid), '-n'] if pid else []
        result = subprocess.run(prefix + args, capture_output=True, text=True, **kwargs)
        if result.returncode:
            raise AssertionError(f'{args}: {result.stderr}')
        return result

    def namespace():
        p = subprocess.Popen(['unshare', '-n', 'sleep', '120'])
        processes.append(p)
        for _ in range(100):
            if os.readlink(f'/proc/{p.pid}/ns/net') != os.readlink('/proc/self/ns/net'):
                return p.pid
            time.sleep(0.01)
        raise AssertionError('namespace did not start')

    def connect(address, port, pid=None):
        current = os.open('/proc/self/ns/net', os.O_RDONLY)
        target = os.open(f'/proc/{pid}/ns/net', os.O_RDONLY) if pid else None
        try:
            if target is not None:
                assert libc.setns(target, 0) == 0
            s = socket.socket(socket.AF_INET6 if ':' in address else socket.AF_INET)
            s.settimeout(0.5)
            sockets.append(s)
            s.connect((address, port))
            return s
        finally:
            assert libc.setns(current, 0) == 0
            os.close(current)
            if target is not None:
                os.close(target)

    def echo(s):
        s.sendall(b'ok')
        assert s.recv(2) == b'ok'

    def allowed(address, port, pid=None):
        try:
            s = connect(address, port, pid)
            echo(s)
            s.close()
        except OSError as exc:
            diagnostics = []
            for target in (None, vps, container):
                for args in (['ip', '-6', 'route'], ['ip', '-6', 'neigh'], ['ip', '-6', 'addr'], ['sysctl', 'net.ipv6.conf.all.forwarding']):
                    diagnostics.append(str(target) + ' ' + run(args, target).stdout)
            raise AssertionError(f'Expected connection {address}:{port} from namespace {pid}: {exc}\n' + '\n'.join(diagnostics)) from exc

    def blocked(address, port, pid=None):
        try:
            s = connect(address, port, pid)
            echo(s)
        except (OSError, AssertionError):
            return
        raise AssertionError(f'Public access unexpectedly allowed: {address}:{port}')

    def udp(address, port, source_port=0, expect_reply=True):
        with socket.socket(socket.AF_INET6 if ':' in address else socket.AF_INET, socket.SOCK_DGRAM) as s:
            s.settimeout(0.3)
            s.bind(('::' if ':' in address else '0.0.0.0', source_port))
            s.sendto(b'ok', (address, port))
            try:
                data, _ = s.recvfrom(64)
            except socket.timeout:
                assert not expect_reply, f'UDP {address}:{port} was unexpectedly blocked'
            else:
                assert expect_reply and data == b'ok', f'UDP {address}:{port} was unexpectedly allowed'

    def server(pid, addresses, ports, program=SERVER):
        command = [sys.executable, '-c', program, json.dumps([(a, p) for a in addresses for p in ports])]
        if pid:
            command = ['nsenter', '-t', str(pid), '-n'] + command
        p = subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        processes.append(p)
        assert p.stdout.readline().strip() == 'ready'

    try:
        vps, container = namespace(), namespace()
        for pid in (None, vps, container):
            run(['ip', 'link', 'set', 'lo', 'up'], pid)
        run(['ip', 'link', 'add', 'internet', 'type', 'veth', 'peer', 'name', 'eth0'])
        run(['ip', 'link', 'set', 'eth0', 'netns', str(vps)])
        run(['ip', 'link', 'add', 'docker0', 'type', 'veth', 'peer', 'name', 'container0'], vps)
        run(['ip', 'link', 'set', 'container0', 'netns', str(container)], vps)
        for pid, iface, v4, v6 in [
            (None, 'internet', '192.0.2.2/24', 'fd00:1::2/64'),
            (vps, 'eth0', '192.0.2.1/24', 'fd00:1::1/64'),
            (vps, 'docker0', '172.18.0.1/24', 'fd00:18::1/64'),
            (container, 'container0', '172.18.0.2/24', 'fd00:18::2/64'),
        ]:
            run(['sysctl', '-w', f'net.ipv6.conf.{iface}.accept_dad=0'], pid)
            run(['ip', 'addr', 'add', v4, 'dev', iface], pid)
            run(['ip', '-6', 'addr', 'add', v6, 'dev', iface, 'nodad'], pid)
            run(['ip', 'link', 'set', iface, 'up'], pid)
        run(['ip', 'route', 'add', '172.18.0.0/24', 'via', '192.0.2.1'])
        run(['ip', '-6', 'route', 'add', 'fd00:18::/64', 'via', 'fd00:1::1'])
        run(['ip', 'route', 'add', 'default', 'via', '172.18.0.1'], container)
        run(['ip', '-6', 'route', 'add', 'default', 'via', 'fd00:18::1'], container)
        run(['sysctl', '-w', 'net.ipv4.ip_forward=1'], vps)
        run(['sysctl', '-w', 'net.ipv6.conf.all.forwarding=1'], vps)
        # Docker-style published port, tested before and after the guard.
        nat = '''table ip docker_test { chain prerouting {
 type nat hook prerouting priority -100;
 ip daddr 192.0.2.1 tcp dport 18080 dnat to 172.18.0.2:8080
}
}
table ip6 docker_test { chain prerouting {
 type nat hook prerouting priority -100;
 ip6 daddr fd00:1::1 tcp dport 18080 dnat to [fd00:18::2]:8080
}
}
'''
        run(['nft', '-f', '-'], vps, input=nat)
        server(vps, ['0.0.0.0', '::'], [22, 80, 139, 445])
        server(vps, ['0.0.0.0', '::'], [68, 137, 138, 546], UDP_SERVER)
        for address in ('192.0.2.1', 'fd00:1::1'):
            for port in (137, 138):
                udp(address, port)
        server(container, ['0.0.0.0', '::'], [22, 8080])
        server(None, ['0.0.0.0', '::'], [9000])
        for address in ('172.18.0.2', 'fd00:18::2'):
            allowed(address, 8080)
        for address in ('192.0.2.1', 'fd00:1::1'):
            for port in (22, 80, 445, 18080):
                allowed(address, port)
        old_http = connect('192.0.2.1', 80)
        old_ssh = connect('192.0.2.1', 22)
        outbound = connect('192.0.2.2', 9000, vps)
        echo(old_http); echo(old_ssh); echo(outbound)
        rules = render({'interfaces': ['eth0'], 'ssh_ports': [22]})
        run(['nft', '--check', '-f', '-'], vps, input=rules)
        run(['nft', '-f', '-'], vps, input=rules)
        run(['nft', '-f', '-'], vps, input=rules)  # idempotent/atomic reload
        echo(old_ssh)
        echo(outbound)
        for address in ('192.0.2.1', 'fd00:1::1'):
            for port in (137, 138):
                udp(address, port, expect_reply=False)
        udp('192.0.2.1', 68, source_port=67)
        udp('fd00:1::1', 546, source_port=547)
        # Force fresh IPv6 neighbor discovery through the public ingress guard.
        run(['ip', '-6', 'neigh', 'flush', 'dev', 'eth0'], vps)
        run(['ip', '-6', 'neigh', 'flush', 'dev', 'internet'])
        try:
            echo(old_http)
        except OSError:
            pass
        else:
            raise AssertionError('Previously established public HTTP was not blocked')
        for address in ('192.0.2.1', 'fd00:1::1'):
            allowed(address, 22)
            for port in (80, 139, 445, 18080):
                blocked(address, port)
        for address in ('172.18.0.2', 'fd00:18::2'):
            blocked(address, 8080)
            blocked(address, 22)  # SSH exception must not allow routed container SSH
            allowed(address, 8080, vps)
        for pid in (vps, container):
            allowed('192.0.2.2', 9000, pid)
            allowed('fd00:1::2', 9000, pid)
        allowed('127.0.0.1', 80, vps)
        allowed('::1', 80, vps)
        run(['nft', 'delete', 'table', 'inet', 'homeprod_guard'], vps)
        allowed('192.0.2.1', 80)
        allowed('fd00:1::1', 18080)
        print('PASS: IPv4/IPv6 SSH, host/Docker blocking, direct container routing,')
        print('      NetBIOS UDP blocked, DHCP/ND preserved, old HTTP blocked,')
        print('      outbound replies, loopback, reload and rollback.')
    finally:
        for s in sockets:
            s.close()
        for p in reversed(processes):
            p.terminate()
            try:
                p.wait(timeout=3)
            except subprocess.TimeoutExpired:
                p.kill(); p.wait()


class NetworkTests(unittest.TestCase):
    def test_packets_in_disposable_namespaces(self):
        for tool in ('unshare', 'nsenter', 'nft', 'ip', 'sysctl'):
            if not shutil.which(tool):
                self.skipTest(f'{tool} unavailable')
        check = subprocess.run(['unshare', '-Urn', 'true'], capture_output=True)
        if check.returncode:
            self.skipTest('Unprivileged user/network namespaces unavailable')
        env = dict(os.environ, HOMEPROD_TEST_NETNS='1', PYTHONDONTWRITEBYTECODE='1')
        result = subprocess.run(['unshare', '-Urn', sys.executable, str(Path(__file__).resolve()), '--inside'],
                                env=env, capture_output=True, text=True, timeout=60)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        print(result.stdout.strip())


if __name__ == '__main__':
    if sys.argv[1:] == ['--inside']:
        packet_checks()
    else:
        unittest.main()
