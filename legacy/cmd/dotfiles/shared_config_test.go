package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSharedConfigTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestConfigPathSharedFallbackAndProfilePriority(t *testing.T) {
	root := t.TempDir()
	shared := filepath.Join(root, "setup", "config", "app", "config")
	writeSharedConfigTestFile(t, shared, "shared")
	profile := setupDirectory(root, deviceDesktop)
	if got := configPath(profile, "app", "config"); got != shared {
		t.Fatalf("base compartilhada não resolvida: %s", got)
	}
	local := filepath.Join(profile, "app", "config")
	writeSharedConfigTestFile(t, local, deviceDesktop.String())
	if got := configPath(profile, "app", "config"); got != local {
		t.Fatalf("override do perfil não teve prioridade: %s", got)
	}
	missing := filepath.Join(profile, "app", "missing")
	if got := configPath(profile, "app", "missing"); got != missing {
		t.Fatalf("caminho ausente incorreto: %s", got)
	}
}
