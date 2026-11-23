package main

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func TestInstallRustUsesStableToolchain(t *testing.T) {
	pacman := "sudo pacman -S --needed -- rustup"
	setStable := "rustup default stable"
	for _, tt := range []struct {
		name, answer, defaultToolchain string
		missing                        bool
		want                           []string
	}{
		{name: "instalação inicial", answer: "CONFIRMO", missing: true, want: []string{pacman, setStable}},
		{name: "estável já configurada", answer: "CONFIRMO", defaultToolchain: "stable-x86_64-unknown-linux-gnu (default)"},
		{name: "rustup existente com nightly", answer: "CONFIRMO", defaultToolchain: "nightly-x86_64-unknown-linux-gnu (default)", want: []string{setStable}},
		{name: "cancelar", answer: "CANCELAR", missing: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			installer, commands := newPackageInstallerTest(t, tt.answer+"\n")
			installer.which = func(name string) (string, error) {
				if name != "rustup" {
					t.Fatalf("executável inesperado: %s", name)
				}
				if tt.missing {
					return "", errors.New("rustup ausente")
				}
				return "/usr/bin/rustup", nil
			}
			var checks []string
			capture := installer.console.commands.capture
			installer.console.commands.capture = func(input string, argv ...string) (string, error) {
				switch strings.Join(argv, " ") {
				case "rustup default":
					return tt.defaultToolchain, nil
				case "rustup run stable rustc --version", "rustup run stable cargo --version":
					checks = append(checks, argv[3])
					return argv[3] + " 1.0.0", nil
				default:
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
