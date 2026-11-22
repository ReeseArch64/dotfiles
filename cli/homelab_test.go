package main

import (
	"bufio"
	"bytes"
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestHomelabMenuRoutesOnlyItsActions(t *testing.T) {
	menu := newMenu("homelab-alan")
	for _, tt := range []struct {
		keys   []string
		action string
	}{
		{[]string{"1", "1"}, "ssh"},
		{[]string{"1", "2"}, "firewall"},
		{[]string{"2"}, "casaos"},
	} {
		current := menu
		for _, key := range tt.keys {
			next, _ := current.handleKey(key)
			current = next.(model)
		}
		items := current.items()
		if len(items) != 1 || items[0].disabled {
			t.Fatalf("submenu de %s: %v", tt.action, items)
		}
		command, err := actionCommand(items[0].action, items[0].title)
		if err != nil || !slices.Equal(command.Args[1:], []string{"--homelab", tt.action}) {
			t.Fatalf("ação %s: %v, %v", tt.action, command, err)
		}
	}
	if ubuntuRelease("ID=debian\nID_LIKE=ubuntu\n") || !ubuntuRelease("NAME=Ubuntu\nID=\"ubuntu\"\n") {
		t.Fatal("detecção do Ubuntu incorreta")
	}
	if err := runHomelabAction(identifyProfile("homeprod"), []string{"ssh"}); err == nil {
		t.Fatal("executou ação do Homelab no HomeProd")
	}
}

func TestHomelabSSHWaitsForAccessTest(t *testing.T) {
	c, key, _ := newSSHConsoleTest(t)
	c.input = bufio.NewReader(strings.NewReader("CONFIRMO\n" + key + "\nCANCELAR\n"))
	var calls []string
	capture := c.commands.capture
	c.commands.capture = func(input string, args ...string) (string, error) {
		if args[0] == "dpkg-query" {
			return "install ok installed", nil
		}
		return capture(input, args...)
	}
	c.commands.interactive = func(args ...string) error {
		calls = append(calls, args[4])
		return nil
	}
	if err := configureHomelabSSH(c, "alan"); err == nil || !strings.Contains(err.Error(), "adicionada para teste") {
		t.Fatalf("teste deve interromper o endurecimento: %v", err)
	}
	if !slices.Equal(calls, []string{"prepare-ssh"}) {
		t.Fatalf("etapas administrativas: %v", calls)
	}
	sdkTestContent(t, filepath.Join(c.home, ".ssh", "authorized_keys"), key+"\n")
}

func TestHardenHomelabSSHPreservesConfigurationAndRollsBack(t *testing.T) {
	for _, scenario := range []string{"reaplicar", "reload falha", "outro usuário"} {
		t.Run(scenario, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "sshd_config")
			original := "Include /etc/ssh/sshd_config.d/*.conf\nPort 2222\n"
			sdkTestFile(t, path, original)
			reloads := 0
			commands := sshCommands{capture: func(_ string, args ...string) (string, error) {
				switch args[0] {
				case "sshd":
					if args[1] == "-T" {
						effective := strings.ToLower(sshPolicy("alan", "2222"))
						if scenario == "outro usuário" {
							effective += "allowusers someone-else\n"
						}
						return effective, nil
					}
				case "systemctl":
					if args[1] == "reload" {
						reloads++
						if scenario == "reload falha" && reloads == 1 {
							return "", errors.New("falha simulada")
						}
					}
				}
				return "", nil
			}}
			backup := func(data []byte) (string, error) { return "backup", nil }
			apply := func() error {
				return hardenHomelabSSH(path, "alan", "192.168.1.4 42000 192.168.1.8 2222", commands, commands, backup)
			}
			if err := apply(); scenario == "reaplicar" {
				if err != nil {
					t.Fatal(err)
				}
				if err := apply(); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(path)
				if err != nil || strings.Count(string(data), homelabSSHBegin) != 1 || !strings.HasSuffix(string(data), original) {
					t.Fatalf("configuração original ou bloco gerenciado incorreto: %s, %v", data, err)
				}
			} else {
				if err == nil || scenario == "reload falha" && reloads != 2 {
					t.Fatalf("falha ou rollback não detectado: %v, recargas: %d", err, reloads)
				}
				sdkTestContent(t, path, original)
			}
		})
	}
}

func TestHomelabFirewallOrdersLANAndCurrentSessionBeforeDefaults(t *testing.T) {
	root := t.TempDir()
	for name, data := range map[string]string{"defaults": "IPV6=yes\n", "ufw.conf": "ENABLED=yes\n", "user.rules": "existing", "user6.rules": "existing-v6"} {
		sdkTestFile(t, filepath.Join(root, name), data)
	}
	var calls []string
	f := firewall{configDir: root, defaults: filepath.Join(root, "defaults"), commands: sshCommands{capture: func(_ string, args ...string) (string, error) {
		calls = append(calls, strings.Join(args, " "))
		return "", nil
	}}}
	_, network, _ := net.ParseCIDR("192.168.1.0/24")
	if err := f.applyHomelab(localNetwork{"eth0", network}, "10.0.0.4 22111 192.168.1.8 2222"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"ufw prepend allow in on eth0 from 192.168.1.0/24 to any comment Homelab LAN",
		"ufw prepend allow from 10.0.0.4 to any port 2222 proto tcp comment SSH admin atual",
		"ufw default deny incoming", "ufw default allow outgoing", "ufw --force enable",
		"systemctl enable --now ufw.service", "systemctl is-active --quiet ufw.service",
	}
	if len(calls) < 2 || !slices.Equal(calls[2:], want) {
		t.Fatalf("regras e ordem incorretas: %v", calls)
	}
	if err := f.applyHomelab(localNetwork{"eth0", network}, "invalid"); err == nil || len(calls) != len(want)+2 {
		t.Fatalf("sessão inválida alterou o firewall: %v, %v", err, calls)
	}
}

func TestCasaOSRunsInstallerOnlyAfterCompleteDownload(t *testing.T) {
	for _, fails := range []bool{false, true} {
		var calls []string
		c := sshConsole{input: bufio.NewReader(strings.NewReader("CONFIRMO\n")), output: &bytes.Buffer{}, commands: sshCommands{
			capture: func(_ string, args ...string) (string, error) {
				if args[0] == "systemctl" {
					return "", errors.New("serviço ausente")
				}
				return "install ok installed", nil
			},
			interactive: func(args ...string) error {
				if args[0] == "curl" {
					calls = append(calls, "download")
					if fails {
						return errors.New("download interrompido")
					}
					return os.WriteFile(args[len(args)-2], []byte("#!/bin/bash\nexit 0\n"), 0600)
				}
				calls = append(calls, strings.Join(args[:len(args)-1], " "))
				if args[1] == "usermod" && args[len(args)-1] != "alan" {
					t.Fatalf("usuário incorreto: %v", args)
				}
				return nil
			},
		}}
		err := installCasaOS(&c, "alan")
		want := []string{"download"}
		if !fails {
			want = append(want, "sudo bash --", "sudo usermod -aG docker --")
		}
		if (err != nil) != fails || !slices.Equal(calls, want) {
			t.Fatalf("falha=%t, erro=%v, etapas=%v", fails, err, calls)
		}
	}
}
