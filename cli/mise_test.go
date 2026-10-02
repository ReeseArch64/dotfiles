package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallMiseConfigCreatesSymlink(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(dotfiles, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".config", "mise"), 0755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dotfiles, "mise.toml")
	if err := os.WriteFile(source, []byte("[tools]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(home, ".config", "mise", "config.toml")
	if err := os.WriteFile(destination, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := installMiseConfig(dotfiles, home); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(destination)
	if err != nil {
		t.Fatal(err)
	}
	if target != source {
		t.Fatalf("destino inesperado: %s", target)
	}
}
