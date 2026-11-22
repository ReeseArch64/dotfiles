package main

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func newPackageInstallerTest(t *testing.T, answers string) (packageInstaller, *[]string) {
	t.Helper()
	var commands []string
	console := &sshConsole{
		home: t.TempDir(), input: bufio.NewReader(strings.NewReader(answers)), output: &bytes.Buffer{},
		commands: sshCommands{
			capture: func(_ string, args ...string) (string, error) {
				if len(args) == 3 && args[0] == "pacman" && args[1] == "-Q" {
					return "", errors.New("pacote não instalado")
				}
				t.Fatalf("comando inesperado: %v", args)
				return "", nil
			},
			interactive: func(args ...string) error {
				commands = append(commands, strings.Join(args, " "))
				return nil
			},
		},
	}
	return packageInstaller{
		console: console,
		which: func(name string) (string, error) {
			if name != "yay" {
				t.Fatalf("executável inesperado: %s", name)
			}
			return "/usr/bin/yay", nil
		},
		build: func(path string) error {
			t.Fatalf("compilação inesperada em %s", path)
			return nil
		},
	}, &commands
}

func TestPackageActionsInstallApprovedPackages(t *testing.T) {
	tests := []struct {
		action string
		want   []string
	}{
		{"essential-tools", []string{"sudo pacman -S --needed -- base-devel curl wget rate-mirrors"}},
		{"terminal-tools", []string{
			"sudo pacman -S --needed -- bat eza btop fzf ripgrep fd jq yq zoxide ncdu tealdeer wl-clipboard atuin starship yazi sshfs glow",
			"yay -S --needed -- viddy",
		}},
		{"proton-pass", []string{"yay -S --needed -- proton-pass-cli-bin"}},
		{"install-yay", nil},
	}
	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			installer, commands := newPackageInstallerTest(t, "CONFIRMO\n")
			if err := installer.run(tt.action, ""); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(*commands, tt.want) {
				t.Fatalf("comandos = %q, esperado %q", *commands, tt.want)
			}
			if _, err := os.Stat(filepath.Join(installer.console.home, ".aur")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("yay disponível deveria evitar a preparação do clone: %v", err)
			}
		})
	}
	installer, commands := newPackageInstallerTest(t, "CANCELAR\n")
	if err := installer.run("terminal-tools", ""); err == nil || len(*commands) != 0 {
		t.Fatalf("cancelamento executou comandos: %v, %v", *commands, err)
	}
}

func TestDriversFindHeadersForInstalledKernels(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		installer, commands := newPackageInstallerTest(t, "CONFIRMO\n")
		modules := t.TempDir()
		kernels := map[string]string{
			"6.18-cachyos": "linux-cachyos",
			"6.12-lts":     "linux-cachyos-lts",
			"6.12.1-lts":   "linux-cachyos-lts",
		}
		for release := range kernels {
			sdkTestFile(t, filepath.Join(modules, release, "vmlinuz"), "imagem de teste")
		}
		sdkTestFile(t, filepath.Join(modules, "outro", "modules.dep"), "")
		capture := installer.console.commands.capture
		installer.console.commands.capture = func(input string, args ...string) (string, error) {
			switch args[1] {
			case "-Qqo":
				return kernels[filepath.Base(filepath.Dir(args[3]))] + "\n", nil
			case "-Si":
				if unavailable && args[3] == "linux-cachyos-headers" {
					return "", errors.New("não consta nos repositórios")
				}
				return "disponível", nil
			default:
				return capture(input, args...)
			}
		}
		err := installer.run("drivers", modules)
		if unavailable {
			if err == nil || !strings.Contains(err.Error(), "linux-cachyos-headers") || len(*commands) != 0 {
				t.Fatalf("headers indisponíveis: %v, %v", *commands, err)
			}
			continue
		}
		want := []string{"sudo pacman -S --needed -- linux-cachyos-lts-headers linux-cachyos-headers vulkan-tools mesa-utils"}
		if err != nil || !slices.Equal(*commands, want) {
			t.Fatalf("headers: %v, %v", *commands, err)
		}
	}
}

func TestYayBootstrap(t *testing.T) {
	for _, state := range []string{"clone novo", "clone existente", "clone alterado", "sem git"} {
		t.Run(state, func(t *testing.T) {
			installer, commands := newPackageInstallerTest(t, "CONFIRMO\n")
			target := filepath.Join(installer.console.home, ".aur", "yay")
			ready := false
			installer.which = func(name string) (string, error) {
				if (name == "git" && state != "sem git") || (name == "yay" && ready) {
					return "/usr/bin/" + name, nil
				}
				return "", exec.ErrNotFound
			}
			createClone := func() {
				if err := os.MkdirAll(filepath.Join(target, ".git"), 0700); err != nil {
					t.Fatal(err)
				}
				sdkTestFile(t, filepath.Join(target, "PKGBUILD"), "")
			}
			if state == "clone existente" || state == "clone alterado" {
				createClone()
			}
			capture, run := installer.console.commands.capture, installer.console.commands.interactive
			installer.console.commands.capture = func(input string, args ...string) (string, error) {
				if args[0] == "git" {
					switch args[3] {
					case "config":
						return yayRepository + "\n", nil
					case "status":
						if state == "clone alterado" {
							return " M PKGBUILD\n", nil
						}
						return "", nil
					}
				}
				return capture(input, args...)
			}
			installer.console.commands.interactive = func(args ...string) error {
				if args[0] == "git" {
					if want := []string{"git", "clone", "--", yayRepository, target}; !slices.Equal(args, want) {
						t.Fatalf("clone = %q, esperado %q", args, want)
					}
					createClone()
				}
				return run(args...)
			}
			installer.build = func(directory string) error {
				if directory != target {
					t.Fatalf("compilação em %s", directory)
				}
				*commands = append(*commands, "makepkg -si")
				ready = true
				return nil
			}
			err := installer.run("install-yay", "")
			switch state {
			case "clone alterado", "sem git":
				if err == nil || len(*commands) != 0 {
					t.Fatalf("instalação deveria falhar: %q, %v", *commands, err)
				}
			default:
				want := []string{"sudo pacman -S --needed -- base-devel", "makepkg -si"}
				if state == "clone novo" {
					want = slices.Insert(want, 1, "git clone -- "+yayRepository+" "+target)
				}
				if err != nil || !slices.Equal(*commands, want) {
					t.Fatalf("instalação = %q, erro %v, esperado %q", *commands, err, want)
				}
			}
		})
	}
}

func TestAURDirectoryUsesSDKLink(t *testing.T) {
	home, sdk := t.TempDir(), t.TempDir()
	destination := filepath.Join(sdk, "Aur")
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".aur")
	if err := os.Symlink(destination, link); err != nil {
		t.Fatal(err)
	}
	path, err := aurDirectory(home, sdk)
	if err != nil || path != link {
		t.Fatalf("pasta AUR = %s, erro = %v", path, err)
	}
	if _, err := aurDirectory(home, t.TempDir()); err == nil {
		t.Fatal("aceitou link para fora da pasta SDK")
	}
}
