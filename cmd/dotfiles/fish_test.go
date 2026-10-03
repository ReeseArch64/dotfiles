package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallFishConfigCopiesFileAndRemovesLegacyActivation(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	source := configPath(dotfiles, "fish", "config.fish")
	if err := os.MkdirAll(filepath.Dir(source), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("mise activate fish | source\n"), 0644); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(home, ".config", "fish", "conf.d", "dotfiles-mise.fish")
	if err := os.MkdirAll(filepath.Dir(legacy), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := installFishConfig(dotfiles, home); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(home, ".config", "fish", "config.fish")
	content, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "mise activate fish | source\n" {
		t.Fatalf("configuração inesperada: %q", content)
	}
	if _, err := os.Lstat(legacy); !os.IsNotExist(err) {
		t.Fatalf("ativação antiga não foi removida: %v", err)
	}
}

func TestRepositoryFishConfigExists(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "fish", "config.fish")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("configuração Fish não é arquivo regular: %s", path)
	}
}
