package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestFastfetchCreatesMissingProfilePhotoAndKeepsCustomPhoto(t *testing.T) {
	home := t.TempDir()
	destination := filepath.Join(home, ".face")
	lock := filepath.Join(home, ".dotfiles-fastfetch.lock")
	source := filepath.Join("..", "cachyos", "fastfetch", "face.jpg")
	want, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := ensureProfilePhoto(source, destination, lock); err != nil {
		t.Fatal(err)
	}
	if photo, err := os.ReadFile(destination); err != nil || !bytes.Equal(want, photo) {
		t.Fatalf("foto aplicada incorretamente: %v", err)
	}
	custom := []byte("foto personalizada")
	if err := os.WriteFile(destination, custom, 0644); err != nil {
		t.Fatal(err)
	}
	if err := ensureProfilePhoto(source, destination, lock); err != nil {
		t.Fatal(err)
	}
	if photo, err := os.ReadFile(destination); err != nil || !bytes.Equal(custom, photo) {
		t.Fatalf("foto personalizada alterada: %v", err)
	}
}

func TestApplyTerminalConfigCopiesAssetsAndBacksUpChanges(t *testing.T) {
	for _, file := range []string{"fish/config.fish", "fastfetch/config.jsonc"} {
		t.Run(file, func(t *testing.T) {
			home := t.TempDir()
			source := filepath.Join("..", "cachyos", file)
			destination := filepath.Join(home, ".config", file)
			lock := filepath.Join(home, ".dotfiles.lock")
			location := backupLocation{path: filepath.Join(home, "backups")}
			want, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			checkFile := func(path string, expected []byte) {
				t.Helper()
				data, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(data, expected) {
					t.Fatalf("conteúdo inesperado em %q: %v", path, err)
				}
			}
			if backup, err := applyTerminalConfig(source, destination, lock, location); err != nil || backup != "" {
				t.Fatalf("primeira cópia: backup %q, erro %v", backup, err)
			}
			checkFile(destination, want)
			other := filepath.Join(filepath.Dir(destination), "local")
			previous := []byte("configuração anterior\n")
			if err := os.WriteFile(other, previous, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(destination, previous, 0644); err != nil {
				t.Fatal(err)
			}
			backup, err := applyTerminalConfig(source, destination, lock, location)
			if err != nil || filepath.Dir(backup) != location.path {
				t.Fatalf("substituição: backup %q, erro %v", backup, err)
			}
			checkFile(backup, previous)
			checkFile(destination, want)
			checkFile(other, previous)
			if backup, err := applyTerminalConfig(source, destination, lock, location); err != nil || backup != "" {
				t.Fatalf("configuração idêntica: backup %q, erro %v", backup, err)
			}
		})
	}
}
