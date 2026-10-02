package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInstallBtopCopiesCompleteDirectory(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	source := configPath(dotfiles, "btop")
	if err := os.MkdirAll(filepath.Join(source, "themes"), 0755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"btop.conf":             "color_theme = \"noctalia\"\n",
		"themes/noctalia.theme": "theme[main_bg]=\"#282a36\"\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(source, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := installBtop(dotfiles, home, time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(home, ".config", "btop")
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

func TestInstallBtopPreservesExistingConfiguration(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	source := configPath(dotfiles, "btop")
	destination := filepath.Join(home, ".config", "btop")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "btop.conf"), []byte("new\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destination, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "btop.conf"), []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	if err := installBtop(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	backup := destination + ".backup-20250102-030405"
	content, err := os.ReadFile(filepath.Join(backup, "btop.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "old\n" {
		t.Fatalf("backup inesperado: %q", content)
	}
}
