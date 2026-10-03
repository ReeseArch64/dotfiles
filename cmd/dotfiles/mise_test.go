package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestInstallMiseConfigCreatesSymlink(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(configPath(dotfiles, "mise"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".config", "mise"), 0755); err != nil {
		t.Fatal(err)
	}
	source := configPath(dotfiles, "mise", "mise.toml")
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

func TestMiseConfigIncludesCloudCLIs(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "configs", "mise", "mise.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`awscli = { version = "latest", symlink_bins = true }`,
		`gcloud = { version = "latest" }`,
		`railway = { version = "latest" }`,
		`firebase = { version = "latest" }`,
		`azure-cli = { version = "latest" }`,
	} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("configuração %q ausente em mise.toml", want)
		}
	}
}

func TestMiseConfigIncludesJavaGradleAndMaven(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "configs", "mise", "mise.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`java = ["temurin-21", "temurin-17", "temurin-11", "temurin-8"]`,
		`gradle = { version = "latest" }`,
		`maven = { version = "latest" }`,
	} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("configuração %q ausente em mise.toml", want)
		}
	}
}

func TestMiseConfigExcludesPHPAndComposer(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "configs", "mise", "mise.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"php", "composer"} {
		if strings.Contains(string(content), unwanted) {
			t.Fatalf("configuração removida %q ainda existe em mise.toml", unwanted)
		}
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
	if steps[len(steps)-1].label != "Confiar na configuração do mise" {
		t.Fatalf("confiança ausente: %s", steps[len(steps)-1].label)
	}
	want := []string{"mise", "trust", "/tmp/dotfiles/configs/mise/mise.toml"}
	if got := commandTail(t, steps[len(steps)-1], len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando de confiança inesperado: %v", got)
	}
}
