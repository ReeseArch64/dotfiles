package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInstallMiseConfigCreatesSymlink(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(dotfiles, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".config", "mise"), 0755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dotfiles, "mise.toml")
	if err := os.WriteFile(source, []byte("[tools]\n"), 0644); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(home, ".config", "mise", "config.toml")
	if err := os.WriteFile(destination, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := installMiseConfig(dotfiles, home); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(destination)
	if err != nil {
		t.Fatal(err)
	}
	if target != source {
		t.Fatalf("destino inesperado: %s", target)
	}
}

func TestInstallMiseFishActivation(t *testing.T) {
	home := t.TempDir()
	if err := installMiseFishActivation(home); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".config", "fish", "conf.d", "dotfiles-mise.fish")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != miseFishActivation {
		t.Fatalf("ativação inesperada:\n%s", content)
	}
}

func TestMiseSetupActivatesToolsAfterConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	steps := miseSetupSteps("/tmp/dotfiles")
	if len(steps) < 2 {
		t.Fatalf("passos insuficientes: %d", len(steps))
	}
	if steps[len(steps)-2].label != "Symlink ~/.config/mise/config.toml" {
		t.Fatalf("configuração ausente: %s", steps[len(steps)-2].label)
	}
	if steps[len(steps)-1].label != "Ativar ferramentas do mise no Fish" {
		t.Fatalf("ativação ausente: %s", steps[len(steps)-1].label)
	}
}
