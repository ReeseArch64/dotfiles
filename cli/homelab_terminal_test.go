package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestHomelabTerminalAppliesServerConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name        string
		fish        bool
		noFastfetch bool
		chshNoOp    bool
		alreadyFish bool
	}{
		{name: "Fastfetch only"},
		{name: "Fish and Fastfetch", fish: true},
		{name: "Fish already default", fish: true, alreadyFish: true},
		{name: "Fastfetch failure preserves Fish login", fish: true, noFastfetch: true},
		{name: "Unchanged login shell is an error", fish: true, chshNoOp: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			config := filepath.Join(home, "xdg")
			t.Setenv("XDG_CONFIG_HOME", config)
			for _, asset := range []string{"ubuntu/homelab/fastfetch/config.jsonc", "ubuntu/homelab/fish/config.fish", "cachyos/fastfetch/face.jpg"} {
				if tc.noFastfetch && asset == "ubuntu/homelab/fastfetch/config.jsonc" {
					continue
				}
				data, err := os.ReadFile(filepath.Join("..", asset))
				if err != nil {
					t.Fatal(err)
				}
				sdkTestFile(t, filepath.Join(home, ".dotfiles", asset), string(data))
			}
			sdkTestFile(t, filepath.Join(home, ".bashrc"), "bash original")
			sdkTestFile(t, filepath.Join(home, ".face"), "foto original")
			var installed []string
			var changedShell bool
			shell := "/bin/bash"
			if tc.alreadyFish {
				shell = homelabFishShell
			}
			var output bytes.Buffer
			c := sshConsole{home: home, input: bufio.NewReader(strings.NewReader("CONFIRMO\n")), output: &output, commands: sshCommands{
				capture: func(_ string, args ...string) (string, error) {
					switch args[0] {
					case "dpkg-query":
						installed = append(installed, args[len(args)-1])
						return "install ok installed", nil
					case "getent":
						if !tc.fish || !slices.Equal(args, []string{"getent", "--", "passwd", "alan"}) {
							t.Fatalf("consulta inesperada: %v", args)
						}
						return "alan:x:1000:1000::" + home + ":" + shell + "\n", nil
					case homelabFishShell, "fastfetch":
						return "versão de teste", nil
					default:
						t.Fatalf("consulta inesperada: %v", args)
						return "", nil
					}
				},
				interactive: func(args ...string) error {
					if !slices.Equal(args, []string{"sudo", "chsh", "-s", "/usr/bin/fish", "--", "alan"}) {
						t.Fatalf("comando inesperado: %v", args)
					}
					changedShell = true
					if !tc.chshNoOp {
						shell = homelabFishShell
					}
					return nil
				},
			}}
			err := configureHomelabTerminal(&c, "alan", tc.fish)
			if tc.chshNoOp {
				if err == nil || !strings.Contains(err.Error(), "ficou registrado") {
					t.Fatalf("erro de shell não detectado: %v", err)
				}
			} else if tc.noFastfetch {
				if err == nil || !strings.Contains(err.Error(), "ler configuração do Fastfetch") {
					t.Fatalf("erro do Fastfetch não detectado: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			var want, files []string
			if tc.fish {
				want = append(want, "fish")
				files = append(files, "fish/config.fish")
			}
			if err == nil {
				want = append(want, "chafa")
				files = append(files, "fastfetch/config.jsonc")
				if tc.fish {
					want = append(want, "bat", "bsdextrautils")
				}
			}
			if !slices.Equal(installed, want) || changedShell != (tc.fish && !tc.alreadyFish) {
				t.Fatalf("pacotes=%v, shell alterado=%t", installed, changedShell)
			}
			if wantFish := tc.fish && !tc.chshNoOp; (shell == homelabFishShell) != wantFish || strings.Contains(output.String(), "confirmado como /usr/bin/fish") != wantFish {
				t.Fatalf("shell=%s, saída=%q", shell, output.String())
			}
			for _, file := range files {
				data, err := os.ReadFile(filepath.Join(home, ".dotfiles", "ubuntu", "homelab", file))
				if err != nil {
					t.Fatal(err)
				}
				sdkTestContent(t, filepath.Join(config, file), string(data))
			}
			if _, statErr := os.Stat(filepath.Join(home, ".hushlogin")); (statErr == nil) != (tc.fish && err == nil) {
				t.Fatalf(".hushlogin: %v", statErr)
			}
			sdkTestContent(t, filepath.Join(home, ".bashrc"), "bash original")
			sdkTestContent(t, filepath.Join(home, ".face"), "foto original")
		})
	}
}

func TestHomelabToolDownloadRejectsMismatchedHash(t *testing.T) {
	download := []byte("fixture")
	commands := sshCommands{interactive: func(args ...string) error {
		return os.WriteFile(args[len(args)-2], download, 0600)
	}}
	asset := homelabToolAsset{"tool.deb", fmt.Sprintf("%x", sha256.Sum256(download))}
	if _, err := downloadHomelabTool(commands, t.TempDir(), fastfetchReleaseURL, asset); err != nil {
		t.Fatal(err)
	}
	asset.sha256 = strings.Repeat("0", 64)
	if path, err := downloadHomelabTool(commands, t.TempDir(), fastfetchReleaseURL, asset); err == nil || path != "" {
		t.Fatalf("download com checksum errado: %s, %v", path, err)
	}
}

func TestHomelabToolsPreservesExistingCommands(t *testing.T) {
	var packages []string
	var links [][]string
	fdInstalled := false
	c := sshConsole{input: bufio.NewReader(strings.NewReader("CONFIRMO\n")), output: &bytes.Buffer{}, commands: sshCommands{
		capture: func(_ string, args ...string) (string, error) {
			if args[0] == "dpkg-query" {
				packages = append(packages, args[len(args)-1])
			} else if !slices.Equal(args, []string{"lazydocker", "--version"}) {
				t.Fatalf("comando inesperado: %v", args)
			}
			return "install ok installed", nil
		},
		interactive: func(args ...string) error {
			links = append(links, args)
			fdInstalled = true
			return nil
		},
	}}
	lookup := func(name string) (string, error) {
		if name == "fd" && !fdInstalled {
			return "", errors.New("ausente")
		}
		return name, nil
	}
	if err := installHomelabTools(&c, lookup); err != nil {
		t.Fatal(err)
	}
	want := []string{"jq", "ripgrep", "fd-find", "fzf", "bat", "git", "ncdu", "curl", "ca-certificates"}
	if !slices.Equal(packages, want) || len(links) != 1 || !slices.Equal(links[0], []string{"sudo", "ln", "-sT", "--", "/usr/bin/fdfind", "/usr/local/bin/fd"}) {
		t.Fatalf("pacotes %v, links %v", packages, links)
	}
}
