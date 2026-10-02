package main

import (
	"slices"
	"testing"
)

func commandTail(t *testing.T, current step, length int) []string {
	t.Helper()
	if current.cmd == nil {
		t.Fatal("esperava passo interativo")
	}
	args := current.cmd().Args
	if len(args) < length {
		t.Fatalf("comando incompleto: %v", args)
	}
	return args[len(args)-length:]
}

func TestPacmanInstallStepUsesNeeded(t *testing.T) {
	current := pacmanInstallStep("git", "docker")
	want := []string{"sudo", "pacman", "-S", "--needed", "git", "docker"}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando Pacman inesperado: %v", got)
	}
}

func TestShellyInstallStepOmitsUnsupportedNeededArgument(t *testing.T) {
	current := shellyInstallStep("aur", "visual-studio-code-bin")
	want := []string{"shelly", "install", "aur", "visual-studio-code-bin"}
	got := commandTail(t, current, len(want))
	if !slices.Equal(got, want) {
		t.Fatalf("comando Shelly inesperado: %v", got)
	}
	if slices.Contains(current.cmd().Args, "--needed") {
		t.Fatal("Shelly não deve receber --needed")
	}
}

func TestShellyInstallStepSupportsStandardRepository(t *testing.T) {
	current := shellyInstallStep("standard", "lazygit")
	want := []string{"shelly", "install", "standard", "lazygit"}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando Shelly standard inesperado: %v", got)
	}
}
