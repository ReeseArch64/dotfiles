package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInstallGhosttyCopiesCompleteDirectory(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	source := configPath(dotfiles, "ghostty")
	if err := os.MkdirAll(filepath.Join(source, "themes"), 0755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"config.ghostty":  "theme = noctalia\n",
		"tab-style.css":   "headerbar {}\n",
		"themes/noctalia": "background = #282a36\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(source, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := installGhostty(dotfiles, home, time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(home, ".config", "ghostty")
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

func TestInstallGhosttyPreservesExistingConfiguration(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	source := configPath(dotfiles, "ghostty")
	destination := filepath.Join(home, ".config", "ghostty")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "config.ghostty"), []byte("new\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destination, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "config.ghostty"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	if err := installGhostty(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	backup := destination + ".backup-20250102-030405"
	content, err := os.ReadFile(filepath.Join(backup, "config.ghostty"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "old\n" {
		t.Fatalf("backup inesperado: %q", content)
	}
}
