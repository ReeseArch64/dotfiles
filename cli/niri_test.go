package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInstallNiriCreatesConfigSymlink(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	source := filepath.Join(dotfiles, "niri")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}

	if err := installNiri(dotfiles, home, time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(home, ".config", "niri")
	target, err := os.Readlink(destination)
	if err != nil {
		t.Fatal(err)
	}
	if target != source {
		t.Fatalf("destino inesperado: %s", target)
	}
}

func TestInstallNiriPreservesExistingConfiguration(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	source := filepath.Join(dotfiles, "niri")
	destination := filepath.Join(home, ".config", "niri")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destination, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "config.kdl"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	if err := installNiri(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	backup := destination + ".backup-20250102-030405"
	content, err := os.ReadFile(filepath.Join(backup, "config.kdl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "old" {
		t.Fatalf("backup inesperado: %q", content)
	}
	if target, err := os.Readlink(destination); err != nil || target != source {
		t.Fatalf("symlink inesperado: target=%q err=%v", target, err)
	}
}

func TestInstallNiriIsIdempotent(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	source := filepath.Join(dotfiles, "niri")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	if err := installNiri(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	if err := installNiri(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	if matches, err := filepath.Glob(filepath.Join(home, ".config", "niri.backup-*")); err != nil || len(matches) != 0 {
		t.Fatalf("backups inesperados: %v, err=%v", matches, err)
	}
}
