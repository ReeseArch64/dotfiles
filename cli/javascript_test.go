package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestJavaScriptInstallStepUsesMiseConfig(t *testing.T) {
	current := javascriptInstallStep("/tmp/dotfiles")
	want := []string{"mise", "--cd", "/tmp/dotfiles", "install", "node", "yarn", "bun", "deno", "pnpm"}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando mise inesperado: %v", got)
	}
}

func TestInstallYarnConfigCreatesHomeSymlink(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(dotfiles, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dotfiles, ".yarnrc")
	if err := os.WriteFile(source, []byte("registry example\n"), 0644); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(home, ".yarnrc")
	if err := os.WriteFile(destination, []byte("old\n"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := installYarnConfig(dotfiles, home); err != nil {
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

func TestNpmLoginStepUsesMiseShim(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	current := npmLoginStep("/tmp/home")
	want := []string{"env", "PATH=/tmp/home/.local/share/mise/shims:/usr/bin", "npm", "login"}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando npm login inesperado: %v", got)
	}
}

func TestJavaScriptJobAuthenticatesAfterInstall(t *testing.T) {
	t.Setenv("HOME", "/tmp/home")
	steps := javascriptJob("/tmp/dotfiles").steps
	if len(steps) < 3 {
		t.Fatalf("passos insuficientes: %d", len(steps))
	}
	if steps[len(steps)-3].label != "Instalar Node.js, npm, Yarn, Bun, Deno e pnpm via mise" {
		t.Fatalf("instalação ausente antes do link: %s", steps[len(steps)-3].label)
	}
	if steps[len(steps)-2].label != "Symlink ~/.yarnrc" {
		t.Fatalf("configuração do Yarn ausente: %s", steps[len(steps)-2].label)
	}
	if steps[len(steps)-1].label != "Autenticar no npm" {
		t.Fatalf("último passo inesperado: %s", steps[len(steps)-1].label)
	}
}
