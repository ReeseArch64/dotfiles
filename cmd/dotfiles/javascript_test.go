package main

import (
	"slices"
	"testing"
)

func TestJavaScriptInstallStepUsesMiseConfig(t *testing.T) {
	current := javascriptInstallStep("/tmp/dotfiles")
	want := []string{"mise", "--cd", "/tmp/dotfiles", "install", "node", "bun", "deno", "pnpm"}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando mise inesperado: %v", got)
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
	if len(steps) < 2 {
		t.Fatalf("passos insuficientes: %d", len(steps))
	}
	if steps[len(steps)-2].label != "Instalar Node.js, npm, Bun, Deno e pnpm via mise" {
		t.Fatalf("instalação ausente antes do login: %s", steps[len(steps)-2].label)
	}
	if steps[len(steps)-1].label != "Autenticar no npm" {
		t.Fatalf("último passo inesperado: %s", steps[len(steps)-1].label)
	}
}
