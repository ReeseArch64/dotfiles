package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInstallObsidianCopiesCompleteDirectory(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	source := filepath.Join(dotfiles, "obsidian")
	if err := os.MkdirAll(filepath.Join(source, "snippets"), 0755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"appearance.json":       "{}\n",
		"snippets/noctalia.css": ".theme-dark {}\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(source, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := installObsidian(dotfiles, home, time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(home, ".obsidian")
	info, err := os.Lstat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("destino não é diretório regular: %v", info.Mode())
	}
	for name, want := range files {
		content, err := os.ReadFile(filepath.Join(destination, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != want {
			t.Fatalf("conteúdo inesperado em %s: %q", name, content)
		}
	}
}

func TestInstallObsidianPreservesExistingSymlink(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	source := filepath.Join(dotfiles, "obsidian")
	destination := filepath.Join(home, ".obsidian")
	vault := filepath.Join(t.TempDir(), "vault")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "appearance.json"), []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(vault, destination); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	if err := installObsidian(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	backup := destination + ".backup-20250102-030405"
	target, err := os.Readlink(backup)
	if err != nil {
		t.Fatal(err)
	}
	if target != vault {
		t.Fatalf("backup inesperado: %s", target)
	}
}
