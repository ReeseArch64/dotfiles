package main

import (
	"slices"
	"testing"
)

func TestFlutterInstallStepUsesMiseConfig(t *testing.T) {
	current := flutterInstallStep("/tmp/dotfiles")
	want := []string{"mise", "--cd", "/tmp/dotfiles", "install", "flutter"}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando mise inesperado: %v", got)
	}
}

func TestFlutterJobInstallsAfterMiseSetup(t *testing.T) {
	t.Setenv("HOME", "/tmp/home")
	steps := flutterJob("/tmp/dotfiles").steps
	if len(steps) < 2 {
		t.Fatalf("passos insuficientes: %d", len(steps))
	}
	if steps[len(steps)-1].label != "Instalar Flutter via mise" {
		t.Fatalf("último passo inesperado: %s", steps[len(steps)-1].label)
	}
}
