package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestEffectiveLaptopSSHPort(t *testing.T) {
	for _, tc := range []struct{ name, output, want, errorText string }{
		{"single", "port 22\n", "22", ""},
		{"duplicates", "port 22\nport 22\n", "22", ""},
		{"normalized", "port 022\nport 22\n", "22", ""},
		{"whitespace", "  port\t2222  \npasswordauthentication no\nport 2222\n", "2222", ""},
		{"distinct", "port 22\nport 2222\n", "", "22, 2222"},
		{"missing", "permitrootlogin no\n", "", "nenhuma porta"},
		{"invalid", "port abc\n", "", "inválida"},
		{"zero", "port 0\n", "", "inválida"},
		{"range", "port 65536\n", "", "inválida"},
		{"malformed", "port 22 2222\n", "", "inválida"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := effectiveLaptopSSHPort(tc.output)
			if tc.errorText != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errorText) {
					t.Fatalf("erro esperado %q, obtido %v", tc.errorText, err)
				}
			} else if err != nil || got != tc.want {
				t.Fatalf("porta obtida %q, erro %v", got, err)
			}
		})
	}
}

func TestLaptopSSHServerApplyAndRollback(t *testing.T) {
	for _, failure := range []string{"", "empty", "inactive", "candidate", "restart", "policy", "port"} {
		t.Run(failure, func(t *testing.T) {
			c, key, _ := newLaptopSSHConsoleTest(t)
			t.Setenv("SSH_CONNECTION", "")
			keys := []string{key}
			if failure == "empty" {
				keys = nil
			}
			if err := writeLaptopAuthorizedKeys(filepath.Join(c.home, ".ssh", "authorized_keys"), keys); err != nil {
				t.Fatal(err)
			}
			directory := t.TempDir()
			config := filepath.Join(directory, "sshd_config")
			original := []byte("Port 2222\nPasswordAuthentication yes\n")
			if err := os.WriteFile(config, original, 0600); err != nil {
				t.Fatal(err)
			}
			var commands [][]string
			restarts := 0
			activeChecks := 0
			capture := func(input string, args ...string) (string, error) {
				commands = append(commands, slices.Clone(args))
				if args[0] == "ssh-keygen" {
					return c.commands.capture(input, args...)
				}
				if args[0] == "systemctl" {
					if args[1] == "is-active" && args[len(args)-1] == "sshd.service" {
						activeChecks++
						if failure == "inactive" && activeChecks == 1 {
							return "", errors.New("inactive")
						}
					}
					if args[len(args)-1] == "sshd.socket" {
						return "", errors.New("inactive")
					}
					if args[1] == "is-enabled" {
						return "disabled\n", errors.New("disabled")
					}
					if args[1] == "restart" {
						restarts++
						if failure == "restart" && restarts == 1 {
							return "", errors.New("restart failed")
						}
					}
					return "", nil
				}
				if args[0] == "sshd" {
					if args[1] == "-t" && slices.Contains(args, "-f") && failure == "candidate" {
						return "", errors.New("invalid candidate")
					}
					if args[1] == "-T" {
						if !slices.Contains(args, "-f") {
							return "port 2222\nport 2222\n", nil
						}
						policy := strings.ToLower(laptopSSHPolicy("alan", "2222"))
						if failure == "policy" {
							policy = strings.ReplaceAll(policy, "permitrootlogin no", "permitrootlogin yes")
						}
						return policy, nil
					}
					return "", nil
				}
				return "", errors.New("unexpected command")
			}
			server := laptopSSHServer{username: "alan", home: c.home, config: config, lockPath: filepath.Join(directory, "lock"), commands: laptopSSHCommands{capture: capture}, probe: func(string) error {
				if failure == "port" {
					return errors.New("porta fechada")
				}
				return nil
			}}
			backup, err := server.apply()
			after, _ := os.ReadFile(config)
			if failure == "" || failure == "empty" || failure == "inactive" {
				if err != nil {
					t.Fatal(err)
				}
				if string(after) != laptopSSHPolicy("alan", "2222") {
					t.Fatalf("política incorreta: %s", after)
				}
				info, _ := os.Stat(config)
				if info.Mode().Perm() != 0644 {
					t.Fatal("política ilegível pela CLI")
				}
			} else {
				if err == nil {
					t.Fatal("erro injetado ignorado")
				}
				if !bytes.Equal(after, original) {
					t.Fatalf("configuração anterior não restaurada: %s", after)
				}
				info, _ := os.Stat(config)
				if info.Mode().Perm() != 0600 {
					t.Fatal("modo original não restaurado")
				}
			}
			backedUp, err := os.ReadFile(filepath.Join(backup, "sshd_config"))
			if err != nil || !bytes.Equal(backedUp, original) {
				t.Fatal("backup original não preservado")
			}
			info, _ := os.Stat(backup)
			if info.Mode().Perm() != 0700 {
				t.Fatal("backup exposto")
			}
			if failure == "restart" && restarts != 2 {
				t.Fatal("rollback não reiniciou serviço anterior")
			}
			for _, argv := range commands {
				if slices.Contains(argv, "bash") || slices.Contains(argv, "python3") {
					t.Fatal("fluxo não pode depender de scripts")
				}
			}
		})
	}
}

func TestLaptopSSHServerRefusesMissingKeysAndSocket(t *testing.T) {
	c, key, _ := newLaptopSSHConsoleTest(t)
	directory := t.TempDir()
	config := filepath.Join(directory, "sshd_config")
	original := []byte("Port 22\n")
	if err := os.WriteFile(config, original, 0600); err != nil {
		t.Fatal(err)
	}
	server := laptopSSHServer{username: "alan", home: c.home, config: config, lockPath: filepath.Join(directory, "lock"), commands: c.commands}
	if _, err := server.apply(); err == nil {
		t.Fatal("servidor configurado sem chaves")
	}
	if err := writeLaptopAuthorizedKeys(filepath.Join(c.home, ".ssh", "authorized_keys"), []string{key}); err != nil {
		t.Fatal(err)
	}
	real := c.commands.capture
	server.commands.capture = func(input string, argv ...string) (string, error) {
		if argv[0] == "systemctl" {
			return "active", nil
		}
		return real(input, argv...)
	}
	if _, err := server.apply(); err == nil || !strings.Contains(err.Error(), "sshd.socket") {
		t.Fatalf("socket ativo aceito: %v", err)
	}
	after, _ := os.ReadFile(config)
	if !bytes.Equal(original, after) {
		t.Fatal("pré-requisito alterou configuração")
	}
}
