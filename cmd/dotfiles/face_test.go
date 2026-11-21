package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallFaceReplacesSymlinkWithCopy(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(assetPath(dotfiles), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatal(err)
	}
	source := assetPath(dotfiles, "face.jpg")
	if err := os.WriteFile(source, []byte("image"), 0644); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(home, ".face")
	if err := os.Symlink(source, destination); err != nil {
		t.Fatal(err)
	}

	if err := installFace(dotfiles, home); err != nil {
		t.Fatal(err)
	}
	if !regularFileMatches(source, destination) {
		t.Fatal("foto de perfil não foi copiada como arquivo regular")
	}
}
