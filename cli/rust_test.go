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
		name, answer, defaultToolchain string
		missing, local                 bool
		want                           []string
	}{
		{name: "instalação inicial", answer: "CONFIRMO", missing: true, want: []string{pacman, setStable}},
		{name: "estável já configurada", answer: "CONFIRMO", defaultToolchain: "stable-x86_64-unknown-linux-gnu (default)"},
		{name: "rustup existente com nightly", answer: "CONFIRMO", defaultToolchain: "nightly-x86_64-unknown-linux-gnu (default)", want: []string{setStable}},
		{name: "rustup do cargo fora do PATH", answer: "CONFIRMO", local: true, defaultToolchain: "stable-x86_64-unknown-linux-gnu (default)"},
		{name: "cancelar", answer: "CANCELAR", missing: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			installer, commands := newPackageInstallerTest(t, tt.answer+"\n")
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
			if (err != nil) != (tt.answer == "CANCELAR") || !slices.Equal(*commands, tt.want) {
				t.Fatalf("comandos = %v, erro = %v", *commands, err)
			}
			if tt.answer == "CONFIRMO" && !slices.Equal(checks, []string{"rustc", "cargo"}) ||
				tt.answer == "CANCELAR" && len(checks) != 0 {
				t.Fatalf("verificações = %v", checks)
			}
		})
	}
}
