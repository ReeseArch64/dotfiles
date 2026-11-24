package main

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestInstallRustUsesStableToolchain(t *testing.T) {
	pacman := "sudo pacman -S --needed -- rustup"
	setStable := "/usr/bin/rustup default stable"
	for _, tt := range []struct {
		name, defaultToolchain string
		missing, local         bool
		want                   []string
	}{
		{name: "instalação inicial", missing: true, want: []string{pacman, setStable}},
		{name: "estável já configurada", defaultToolchain: "stable-x86_64-unknown-linux-gnu (default)"},
		{name: "rustup existente com nightly", defaultToolchain: "nightly-x86_64-unknown-linux-gnu (default)", want: []string{setStable}},
		{name: "rustup do cargo fora do PATH", local: true, defaultToolchain: "stable-x86_64-unknown-linux-gnu (default)"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			installer, commands := newPackageInstallerTest(t, "")
			local := filepath.Join(installer.console.home, ".cargo", "bin", "rustup")
			rustup := "/usr/bin/rustup"
			if tt.local {
				rustup = local
			}
			installer.which = func(name string) (string, error) {
				switch {
				case name == local && tt.local:
					return local, nil
				case name == "rustup" && !tt.missing && !tt.local,
					name == "/usr/bin/rustup" && slices.Contains(*commands, pacman):
					return "/usr/bin/rustup", nil
				}
				return "", errors.New("rustup ausente")
			}
			var checks []string
			capture := installer.console.commands.capture
			installer.console.commands.capture = func(input string, argv ...string) (string, error) {
				switch strings.Join(argv, " ") {
				case rustup + " default":
					return tt.defaultToolchain, nil
				case rustup + " run stable rustc --version", rustup + " run stable cargo --version":
					checks = append(checks, argv[3])
					return argv[3] + " 1.0.0", nil
				default:
					if tt.local {
						t.Fatalf("Rustup existente em %s: consulta indevida %v", local, argv)
					}
					return capture(input, argv...)
				}
			}
			err := installer.run("install-rust", "")
			if err != nil || !slices.Equal(*commands, tt.want) {
				t.Fatalf("comandos = %v, erro = %v", *commands, err)
			}
			if !slices.Equal(checks, []string{"rustc", "cargo"}) {
				t.Fatalf("verificações = %v", checks)
			}
		})
	}
}
