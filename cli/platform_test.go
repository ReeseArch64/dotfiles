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
