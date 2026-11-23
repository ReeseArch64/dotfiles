package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestConfigureMiseCopiesConfigBeforeInstallingTools(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join("..", "cachyos", "mise", "mise.toml")
	destination := filepath.Join(home, "custom-config", "mise", "config.toml")
	location := backupLocation{path: filepath.Join(home, "backups")}
	previous := filepath.Join(home, "legacy-mise.toml")
	sdkTestFile(t, previous, "[tools]\nnode = '20'\n")
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(previous, destination); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	installer, calls := newPackageInstallerTest(t, "")
	installer.console.home = home
	var output bytes.Buffer
	installer.console.output = &output
	run := installer.console.commands.interactive
	mise := filepath.Join(home, ".local", "bin", "mise")
	lookup := func(name string) (string, error) {
		if name == mise {
			return mise, nil
		}
		return "", exec.ErrNotFound
	}
	installer.console.commands.interactive = func(args ...string) error {
		if args[0] == mise {
			contents, err := os.ReadFile(destination)
			if err != nil || !bytes.Equal(contents, want) {
				t.Fatalf("configuração indisponível antes do mise trust: %v", err)
			}
		}
		return run(args...)
	}
	if err := configureMise(*installer.console, source, destination, location, lookup); err != nil {
		t.Fatal(err)
	}
	commands := []string{
		"sudo pacman -S --needed -- " + strings.Join(misePackages, " "),
		mise + " trust " + destination,
		"env MISE_GLOBAL_CONFIG_FILE=" + destination + " " + mise + " --cd / install",
	}
	if !slices.Equal(*calls, commands) {
		t.Fatalf("comandos = %q, esperados %q", *calls, commands)
	}
	backups, err := filepath.Glob(filepath.Join(location.path, "mise-*", "config.toml"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backup: %v, %v", backups, err)
	}
	if link, err := os.Readlink(backups[0]); err != nil || link != previous {
		t.Fatalf("link anterior: %q, %v", link, err)
	}
	sdkTestContent(t, previous, "[tools]\nnode = '20'\n")
	if info, err := os.Lstat(destination); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("destino não é arquivo regular: %v", err)
	}
}

func TestConfigureMiseStopsWhenPackageInstallationFails(t *testing.T) {
	installer, _ := newPackageInstallerTest(t, "")
	failure := errors.New("instalação recusada")
	installer.console.commands.interactive = func(args ...string) error {
		if args[0] != "sudo" {
			t.Fatalf("comando inesperado após a falha: %v", args)
		}
		return failure
	}
	source := filepath.Join("..", "cachyos", "mise", "mise.toml")
	destination := filepath.Join(installer.console.home, ".config", "mise", "config.toml")
	lookup := func(string) (string, error) { return "", exec.ErrNotFound }
	if err := configureMise(*installer.console, source, destination, backupLocation{}, lookup); !errors.Is(err, failure) {
		t.Fatalf("erro = %v, esperado %v", err, failure)
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a configuração foi aplicada antes dos pacotes: %v", err)
	}
}
