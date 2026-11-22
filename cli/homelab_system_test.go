package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestHomelabDNSPreservesResolverAndRollsBack(t *testing.T) {
	for _, scenario := range []string{"aplicar duas vezes", "drop-in conflitante", "sem upstream", "restart falha"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			d := homelabDNS{
				config: filepath.Join(root, "resolved.conf"), resolver: filepath.Join(root, "resolv.conf"),
				stub: filepath.Join(root, "stub-resolv.conf"), upstream: filepath.Join(root, "upstream.conf"),
				dropIns: []string{filepath.Join(root, "resolved.conf.d")},
			}
			original := "[Resolve]\nDNS=192.168.1.1\nDNSStubListener=yes\n[Other]\nKey=enabled\n"
			sdkTestFile(t, d.config, original)
			sdkTestFile(t, d.stub, "nameserver 127.0.0.53\n")
			if scenario != "sem upstream" {
				sdkTestFile(t, d.upstream, "nameserver 192.168.1.1\n")
			}
			stubLink := filepath.Base(d.stub)
			if err := os.Symlink(stubLink, d.resolver); err != nil {
				t.Fatal(err)
			}
			if scenario == "drop-in conflitante" {
				sdkTestFile(t, filepath.Join(d.dropIns[0], "90-conflict.conf"), "[Resolve]\nDNSStubListener=udp\n")
			}
			backups, restarts := 0, 0
			commands := sshCommands{capture: func(_ string, args ...string) (string, error) {
				if args[1] == "restart" {
					restarts++
					if scenario == "restart falha" && restarts == 1 {
						return "", errors.New("falha simulada")
					}
				}
				return "", nil
			}}
			save := func(backup homelabBackup) (string, error) {
				backups++
				if string(backup.Contents) != original || backup.ResolverLink != stubLink {
					t.Fatalf("backup incorreto: %v", backup)
				}
				return "backup", nil
			}
			apply := func() error { return d.apply(commands, commands, save) }
			if err := apply(); scenario == "aplicar duas vezes" {
				if err != nil {
					t.Fatal(err)
				}
				if err := apply(); err != nil {
					t.Fatal(err)
				}
				sdkTestContent(t, d.config, "[Resolve]\nDNSStubListener=no\nDNS=192.168.1.1\n[Other]\nKey=enabled\n")
				if link, err := os.Readlink(d.resolver); err != nil || link != d.upstream || backups != 1 || restarts != 2 {
					t.Fatalf("link=%s, backups=%d, reinícios=%d, erro=%v", link, backups, restarts, err)
				}
			} else {
				if err == nil {
					t.Fatal("falha não detectada")
				}
				sdkTestContent(t, d.config, original)
				if link, err := os.Readlink(d.resolver); err != nil || link != stubLink {
					t.Fatalf("link original não preservado: %s, %v", link, err)
				}
				if scenario == "restart falha" && (backups != 1 || restarts != 2) || scenario != "restart falha" && (backups != 0 || restarts != 0) {
					t.Fatalf("backups=%d, reinícios=%d", backups, restarts)
				}
			}
		})
	}
}

func TestHomelabSwapPersistsOnlyAfterActivation(t *testing.T) {
	for _, scenario := range []string{"reaplicar", "swapon falha", "recarga falha", "entrada conflitante"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			path, fstab := filepath.Join(root, "swapfile-homelab"), filepath.Join(root, "fstab")
			original := "# tabela original\n/swap-anterior none swap sw 0 0\n"
			if scenario == "entrada conflitante" {
				original += path + " none ext4 defaults 0 0\n"
			}
			sdkTestFile(t, fstab, original)
			if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
				t.Fatal(err)
			}
			backups, activations, reloads := 0, 0, 0
			commands := sshCommands{capture: func(_ string, args ...string) (string, error) {
				switch args[0] {
				case "findmnt":
					return "ext4\n", nil
				case "blkid":
					return "swap\n", nil
				case "swapon":
					if args[1] == "--show=NAME" {
						if activations > 0 {
							return path + "\n", nil
						}
						return "/swap-anterior\n", nil
					}
					sdkTestContent(t, fstab, original)
					activations++
					if scenario == "swapon falha" {
						return "", errors.New("falha simulada")
					}
				case "systemctl":
					reloads++
					if scenario == "recarga falha" && reloads == 1 {
						return "", errors.New("falha simulada")
					}
				default:
					t.Fatalf("comando inesperado: %v", args)
				}
				return "", nil
			}}
			save := func(data homelabBackup) (string, error) {
				backups++
				if string(data.Contents) != original {
					t.Fatalf("fstab original incorreto: %q", data.Contents)
				}
				return "backup", nil
			}
			apply := func() error { return applyHomelabSwap(context.Background(), fstab, path, 7, commands, commands, save) }
			if err := apply(); scenario == "reaplicar" {
				if err != nil {
					t.Fatal(err)
				}
				if err := apply(); err != nil {
					t.Fatal(err)
				}
				sdkTestContent(t, fstab, original+path+" none swap sw,nofail 0 0\n")
				if activations != 1 || backups != 1 || reloads != 2 {
					t.Fatalf("ativações=%d, backups=%d, recargas=%d", activations, backups, reloads)
				}
			} else {
				if err == nil {
					t.Fatal("falha não detectada")
				}
				sdkTestContent(t, fstab, original)
				want := map[string][]int{"swapon falha": {1, 1, 0}, "recarga falha": {1, 1, 2}, "entrada conflitante": {0, 0, 0}}
				if got := []int{activations, backups, reloads}; !slices.Equal(got, want[scenario]) {
					t.Fatalf("operações inesperadas: %v", got)
				}
			}
		})
	}
}

func TestDisableDNSStubKeepsOtherSettings(t *testing.T) {
	data := []byte("# comment\n[Resolve]\nDNS=1.1.1.1\nDNSStubListener= \\\nyes\n[Resolve]\nDNSStubListener=tcp\n[Other]\nDNSStubListener=udp\n")
	want := "# comment\n[Resolve]\nDNSStubListener=no\nDNS=1.1.1.1\n[Resolve]\n[Other]\nDNSStubListener=udp\n"
	if got := string(disableDNSStub(data)); got != want {
		t.Fatalf("configuração inesperada: %q", got)
	}
	if got := string(disableDNSStub([]byte("# notas"))); !strings.HasSuffix(got, "\n[Resolve]\nDNSStubListener=no\n") {
		t.Fatalf("seção Resolve ausente: %q", got)
	}
}

func TestCreateHomelabSwapFileFormatsAndPublishes(t *testing.T) {
	for _, command := range []string{"mkswap", "blkid"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skipf("%s indisponível", command)
		}
	}
	const size = 1 << 20
	path := filepath.Join(t.TempDir(), "swapfile-homelab")
	commands := systemSSHCommands()
	if err := createHomelabSwapFile(context.Background(), path, size, commands); err != nil {
		t.Fatal(err)
	}
	if exists, err := checkHomelabSwapFile(path, size, commands); err != nil || !exists {
		t.Fatalf("swap publicada inválida: %v", err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("modo incorreto no arquivo de swap: %v, %v", info, err)
	}
}
