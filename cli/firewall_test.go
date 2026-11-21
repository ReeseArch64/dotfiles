package main

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSelectLANAndProtectRemoteSession(t *testing.T) {
	_, private, _ := net.ParseCIDR("192.168.1.0/24")
	_, other, _ := net.ParseCIDR("10.0.0.0/24")
	_, mixed, _ := net.ParseCIDR("192.168.0.0/15")
	if !privateIPv4Subnet(private) || privateIPv4Subnet(mixed) {
		t.Fatal("classificou incorretamente LAN privada")
	}
	lan, err := selectLAN([]localNetwork{{"eth0", private}, {"wifi", other}}, "eth0")
	if err != nil || lan.device != "eth0" {
		t.Fatalf("LAN selecionada: %v; %v", lan, err)
	}
	if err := lan.checkConnection("192.168.1.9 12345 192.168.1.8 22"); err != nil {
		t.Fatal(err)
	}
	for _, connection := range []string{"10.0.0.8 12345 192.168.1.8 22", "invalid"} {
		if err := lan.checkConnection(connection); err == nil {
			t.Fatalf("sessão aceita fora da LAN: %s", connection)
		}
	}
	if _, err := selectLAN([]localNetwork{{"eth0", private}, {"wifi", other}}, ""); err == nil {
		t.Fatal("aceitou LAN ambígua")
	}
}

func TestFirewallRuleOrderAndRollback(t *testing.T) {
	for _, failure := range []string{"", "enable"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			defaults := filepath.Join(root, "defaults")
			files := map[string]string{
				defaults:                           "IPV6=yes\n",
				filepath.Join(root, "ufw.conf"):    "ENABLED=yes\n",
				filepath.Join(root, "user.rules"):  "original rules v4",
				filepath.Join(root, "user6.rules"): "original rules v6",
			}
			for path, data := range files {
				if err := os.WriteFile(path, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			var called []string
			show := 0
			commands := sshCommands{capture: func(_ string, args ...string) (string, error) {
				command := strings.Join(args, " ")
				called = append(called, command)
				switch command {
				case "ufw show added":
					show++
					if show == 1 {
						return "ufw allow 443/tcp\nufw deny 22/tcp comment '" + ufwSSHDeny + "'\n", nil
					}
					return "ufw allow in on eth0 from 192.168.1.0/24 to any port 2222 proto tcp comment '" + ufwSSHAllow +
						"'\nufw deny 2222/tcp comment '" + ufwSSHDeny + "'\nufw allow 443/tcp", nil
				case "ufw --force enable":
					if err := os.WriteFile(defaults, []byte("modified"), 0600); err != nil {
						t.Fatal(err)
					}
					if failure != "" {
						return "", errors.New("injected failure")
					}
				}
				return "", nil
			}}
			_, subnet, _ := net.ParseCIDR("192.168.1.0/24")
			f := firewall{configDir: root, defaults: defaults, commands: commands}
			err := f.apply(localNetwork{"eth0", subnet}, "2222")
			if (err == nil) != (failure == "") {
				t.Fatalf("resultado inesperado: %v", err)
			}
			deny := slices.Index(called, "ufw prepend deny 2222/tcp comment "+ufwSSHDeny)
			allow := slices.Index(called, "ufw prepend allow in on eth0 proto tcp from 192.168.1.0/24 to any port 2222 comment "+ufwSSHAllow)
			if deny < 0 || allow != deny+1 {
				t.Fatalf("ordem de regras incorreta: %v", called)
			}
			if !slices.Contains(called, "ufw --force delete deny 22/tcp comment "+ufwSSHDeny) ||
				slices.Contains(called, "ufw --force delete allow 443/tcp") {
				t.Fatalf("regra antiga não tratada corretamente: %v", called)
			}
			if failure != "" {
				if data, err := os.ReadFile(defaults); err != nil || string(data) != files[defaults] || !slices.Contains(called, "ufw reload") {
					t.Fatalf("configuração anterior não foi restaurada: %s, %v, %v", data, err, called)
				}
			}
		})
	}
}
