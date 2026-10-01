package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestValidateZenBrowserConfigRequiresDirectory(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".config", "zen-browser")

	err := validateZenBrowserConfig(home)
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("erro inesperado: %v", err)
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := validateZenBrowserConfig(home); err != nil {
		t.Fatal(err)
	}
}

func TestZenBrowserPrerequisitesInstallWithShellyBeforeValidation(t *testing.T) {
	steps := zenBrowserPrerequisiteStepsFor(t.TempDir(), []string{zenBrowserPackage})
	if len(steps) != 2 {
		t.Fatalf("quantidade de passos: recebeu %d, esperava 2", len(steps))
	}
	want := []string{"shelly", "install", "aur", zenBrowserPackage}
	if got := commandTail(t, steps[0], len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando Shelly inesperado: %v", got)
	}
	if steps[1].label != "Verificar ~/.config/zen-browser" || steps[1].run == nil {
		t.Fatalf("passo de validação inesperado: %#v", steps[1])
	}
}

func TestZenBrowserPrerequisitesSkipInstalledPackage(t *testing.T) {
	steps := zenBrowserPrerequisiteStepsFor(t.TempDir(), nil)
	if len(steps) != 1 || steps[0].label != "Verificar ~/.config/zen-browser" {
		t.Fatalf("passos inesperados: %#v", steps)
	}
}
