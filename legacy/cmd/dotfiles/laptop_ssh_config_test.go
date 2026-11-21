package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestConfiguredLaptopSSHPort(t *testing.T) {
	for _, tc := range []struct {
		name, contents, include, want string
		fail                          bool
	}{
		{name: "default", contents: "# Port 2222\nPasswordAuthentication yes\n", want: "22"},
		{name: "explicit", contents: "Port=2222 # comment\n", want: "2222"},
		{name: "include", contents: "Include \"conf with spaces/*.conf\"\nPort 2222\n", include: "Port = 2222\n", want: "2222"},
		{name: "include-port", contents: "Include \"conf with spaces/*.conf\"\n", include: "Port 2200\n", want: "2200"},
		{name: "ambiguous", contents: "Port 22\nInclude \"conf with spaces/*.conf\"\n", include: "Port 2200\n", fail: true},
		{name: "cycle", contents: "Include sshd_config\n", fail: true},
		{name: "invalid", contents: "Port invalid\n", fail: true},
		{name: "bad-quote", contents: "Include \"oops\n", fail: true},
		{name: "unsupported-expansion", contents: "Include $HOME/config\n", fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			directory := t.TempDir()
			config := filepath.Join(directory, "sshd_config")
			if err := os.WriteFile(config, []byte(tc.contents), 0600); err != nil {
				t.Fatal(err)
			}
			if tc.include != "" {
				dir := filepath.Join(directory, "conf with spaces")
				if err := os.Mkdir(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "port.conf"), []byte(tc.include), 0600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := configuredLaptopSSHPort(config)
			if tc.fail {
				if err == nil {
					t.Fatalf("configuração inválida aceita: %s", got)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("porta %q, erro %v; esperado %s", got, err, tc.want)
			}
		})
	}
	if _, err := configuredLaptopSSHPort(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("arquivo ausente não pode ser presumido como porta 22")
	}
}

func TestDumpLaptopSSHConfig(t *testing.T) {
	for _, tc := range []struct {
		name, first, second string
		firstError, fail    bool
	}{
		{name: "normal", first: "port 22\n"},
		{name: "equals", first: "Port=2222\nPermitRootLogin=no\n"},
		{name: "empty-T", second: "port 22\n"},
		{name: "warning-T", first: "warning: unavailable\n", second: "port 22\n"},
		{name: "failed-T", firstError: true, second: "port 22\n"},
		{name: "both-empty", fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var modes []string
			commands := laptopSSHCommands{capture: func(_ string, argv ...string) (string, error) {
				if !slices.Equal(argv[:3], []string{"sudo", "-n", "sshd"}) {
					t.Fatalf("prefixo alterado: %v", argv)
				}
				if !slices.Equal(argv[4:], []string{"-f", "candidate"}) {
					t.Fatalf("argumentos alterados: %v", argv)
				}
				modes = append(modes, argv[3])
				if argv[3] == "-T" {
					if tc.firstError {
						return "", errors.New("test error")
					}
					return tc.first, nil
				}
				return tc.second, nil
			}}
			output, err := dumpLaptopSSHConfig(commands, []string{"sudo", "-n", "sshd"}, "-f", "candidate")
			if tc.fail {
				if err == nil || !strings.Contains(err.Error(), "-T") || !strings.Contains(err.Error(), "-G") {
					t.Fatalf("sem diagnóstico dos dois modos: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output, "port ") {
				t.Fatalf("saída não normalizada: %q", output)
			}
			if (len(modes) == 2) != (tc.first == "" || tc.firstError || tc.name == "warning-T") {
				t.Fatalf("modos inesperados: %v", modes)
			}
		})
	}
}

func TestVerifyLaptopSSHPolicyFormatsAndEmptyOutput(t *testing.T) {
	policy := strings.ReplaceAll(laptopSSHPolicy("alan", "22"), " ", "=")
	if err := verifyLaptopSSHPolicy(policy, "alan"); err != nil {
		t.Fatal(err)
	}
	if err := verifyLaptopSSHPolicy("", "alan"); err == nil {
		t.Fatal("política vazia não pode ser considerada segura")
	}
}
