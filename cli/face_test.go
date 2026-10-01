package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallFaceCreatesHomeSymlink(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(dotfiles, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dotfiles, ".face")
	if err := os.WriteFile(source, []byte("image"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".face"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := installFace(dotfiles, home); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(filepath.Join(home, ".face"))
	if err != nil {
		t.Fatal(err)
	}
	if target != source {
		t.Fatalf("destino inesperado: %s", target)
	}
}
