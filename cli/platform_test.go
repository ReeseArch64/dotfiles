package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckPlatformAcceptsRequiredStack(t *testing.T) {
	osRelease := filepath.Join(t.TempDir(), "os-release")
	if err := os.WriteFile(osRelease, []byte("ID=cachyos\n"), 0644); err != nil {
		t.Fatal(err)
	}
	lookPath := func(name string) (string, error) { return "/usr/bin/" + name, nil }

	if err := checkPlatform(osRelease, lookPath); err != nil {
		t.Fatal(err)
	}
}

func TestCheckPrerequisitesAcceptsKeysAndZen(t *testing.T) {
	dotfiles := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".config", "zen-browser"), 0755); err != nil {
		t.Fatal(err)
	}
	keys := map[string]string{
		"minha_chave_privada.asc": "-----BEGIN PGP PRIVATE KEY BLOCK-----\nprivate\n",
		"minha_chave_publica.asc": "-----BEGIN PGP PUBLIC KEY BLOCK-----\npublic\n",
	}
	for name, content := range keys {
		if err := os.WriteFile(filepath.Join(dotfiles, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	lookPath := func(name string) (string, error) {
		if name != "zen-browser" {
			t.Fatalf("executável inesperado: %s", name)
		}
		return "/usr/bin/zen-browser", nil
	}

	if err := checkPrerequisites(dotfiles, home, lookPath); err != nil {
		t.Fatal(err)
	}
}

func TestCheckPrerequisitesReportsEveryMissingRequirement(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	lookPath := func(string) (string, error) { return "", errors.New("não encontrado") }

	err := checkPrerequisites(dotfiles, home, lookPath)
	if err == nil {
		t.Fatal("esperava erro para pré-requisitos ausentes")
	}
	for _, requirement := range []string{"Zen Browser", ".config/zen-browser", "minha_chave_privada.asc", "minha_chave_publica.asc"} {
		if !strings.Contains(err.Error(), requirement) {
			t.Fatalf("erro não menciona %s: %v", requirement, err)
		}
	}
}

func TestCheckPrerequisitesRejectsInvalidKeyFiles(t *testing.T) {
	dotfiles := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".config", "zen-browser"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"minha_chave_privada.asc", "minha_chave_publica.asc"} {
		if err := os.WriteFile(filepath.Join(dotfiles, name), []byte("não é uma chave"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	lookPath := func(string) (string, error) { return "/usr/bin/zen-browser", nil }

	err := checkPrerequisites(dotfiles, home, lookPath)
	if err == nil || !strings.Contains(err.Error(), "ausente ou inválido") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestCheckPlatformRejectsMissingRequirements(t *testing.T) {
	osRelease := filepath.Join(t.TempDir(), "os-release")
	if err := os.WriteFile(osRelease, []byte("ID=arch\n"), 0644); err != nil {
		t.Fatal(err)
	}
	lookPath := func(name string) (string, error) {
		return "", errors.New("não encontrado")
	}

	err := checkPlatform(osRelease, lookPath)
	if err == nil {
		t.Fatal("esperava ambiente incompatível")
	}
	for _, requirement := range []string{"CachyOS", "Niri", "Noctalia Shell"} {
		if !strings.Contains(err.Error(), requirement) {
			t.Fatalf("erro não menciona %s: %v", requirement, err)
		}
	}
}
