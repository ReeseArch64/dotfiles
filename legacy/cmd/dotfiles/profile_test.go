package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDesktopResolvesOnlyRemainingConfiguration(t *testing.T) {
	setupDir := setupDirectory(filepath.Join("..", ".."), deviceDesktop)
	for _, name := range []string{
		"cursor/FrierenBLZ", "fastfetch/config.json", "fish/config.fish", "git/.gitignore",
		"mise/mise.toml", "niri/cfg/misc.kdl", "noctalia/settings.toml", "pi/agent/settings.json",
		"ssh/config", "obsidian", "turing-smart-screen-python", "discord",
	} {
		expected := filepath.Join(setupDir, name)
		if path := configPath(setupDir, name); path != expected {
			t.Fatalf("Desktop legado ainda herda %s de %s", name, path)
		}
		if _, err := os.Lstat(expected); !os.IsNotExist(err) {
			t.Fatalf("configuração migrada ainda está no Desktop legado: %s (%v)", expected, err)
		}
	}
}

func TestFindSetupRepository(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "bin", "nested")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "setup"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "setup"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if got := findSetupRepository(nested); got != root {
		t.Fatalf("repositório incorreto: %s", got)
	}
	t.Setenv("DOTFILES_DIR", root)
	if got := findDotfiles(); got != root {
		t.Fatalf("DOTFILES_DIR ignorado: %s", got)
	}
}
