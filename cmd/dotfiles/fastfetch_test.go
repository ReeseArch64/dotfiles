package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallFastfetchCopiesConfigAndPreservesOtherFiles(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	source := configPath(dotfiles, "fastfetch", "config.json")
	destinationDirectory := filepath.Join(home, ".config", "fastfetch")
	if err := os.MkdirAll(filepath.Dir(source), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("{\"modules\":[\"title\"]}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(destinationDirectory, "themes"), 0755); err != nil {
		t.Fatal(err)
	}
	theme := filepath.Join(destinationDirectory, "themes", "custom.jsonc")
	if err := os.WriteFile(theme, []byte("existing theme\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := installFastfetch(dotfiles, home); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(destinationDirectory, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "{\"modules\":[\"title\"]}\n" {
		t.Fatalf("configuração inesperada: %q", content)
	}
	content, err = os.ReadFile(theme)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "existing theme\n" {
		t.Fatalf("tema existente alterado: %q", content)
	}
}
