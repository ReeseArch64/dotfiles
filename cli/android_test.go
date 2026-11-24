package main

import (
	"errors"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func TestInstallAndroidCLIReusesWorkingLocalBinary(t *testing.T) {
	installer, commands := newPackageInstallerTest(t, "")
	binary := filepath.Join(installer.console.home, ".local", "bin", "android")
	installer.which = func(name string) (string, error) {
		if name == binary {
			return binary, nil
		}
		return "", exec.ErrNotFound
	}
	installer.console.commands.capture = func(_ string, args ...string) (string, error) {
		if !slices.Equal(args, []string{binary, "--version"}) {
			t.Fatalf("verificação inesperada: %v", args)
		}
		return "1.0.0", nil
	}
	if err := installer.run("install-android-cli", ""); err != nil || len(*commands) != 0 {
		t.Fatalf("instalação existente executou %v: %v", *commands, err)
	}
}

func TestInstallAndroidCLIChecksResultAndPropagatesFailures(t *testing.T) {
	for _, scenario := range []string{"instalado", "falha no download", "binário ausente"} {
		t.Run(scenario, func(t *testing.T) {
			installer, commands := newPackageInstallerTest(t, "")
			binary := filepath.Join(installer.console.home, ".local", "bin", "android")
			installer.which = func(string) (string, error) { return "", exec.ErrNotFound }
			verified := false
			installer.console.commands.capture = func(_ string, args ...string) (string, error) {
				switch {
				case slices.Equal(args, []string{"pacman", "-Q", "curl"}):
					return "curl", nil
				case slices.Equal(args, []string{binary, "--version"}):
					verified = true
					if scenario == "binário ausente" {
						return "", exec.ErrNotFound
					}
					return "1.0.0", nil
				default:
					t.Fatalf("comando inesperado: %v", args)
					return "", nil
				}
			}
			if scenario == "falha no download" {
				installer.console.commands.interactive = func(args ...string) error {
					if !slices.Equal(args, []string{"bash", "-o", "pipefail", "-c", androidCLIInstallCommand}) {
						t.Fatalf("comando inesperado: %v", args)
					}
					return errors.New("download falhou")
				}
			}
			err := installer.run("install-android-cli", "")
			if (err == nil) != (scenario == "instalado") || verified != (scenario != "falha no download") {
				t.Fatalf("verificou binário = %v, erro = %v", verified, err)
			}
			if scenario != "falha no download" && !slices.Equal(*commands, []string{"bash -o pipefail -c " + androidCLIInstallCommand}) {
				t.Fatalf("comandos = %v", *commands)
			}
		})
	}
}
