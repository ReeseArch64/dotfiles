package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func prepareNoctaliaTest(t *testing.T) (string, string, string) {
	t.Helper()
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	sourceDir := filepath.Join(dotfiles, "noctalia")
	pluginDir := filepath.Join(home, ".local", "state", "noctalia", "plugins", "materialized", "community")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"settings.toml", "state.toml"} {
		if err := os.WriteFile(filepath.Join(sourceDir, name), []byte("new "+name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dotfiles, home, pluginDir
}

func installNoctaliaTestPlugins(t *testing.T, pluginDir string) {
	t.Helper()
	for _, name := range requiredNoctaliaPlugins {
		if err := os.Mkdir(filepath.Join(pluginDir, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInstallNoctaliaRequiresPlugins(t *testing.T) {
	dotfiles, home, pluginDir := prepareNoctaliaTest(t)
	if err := os.Mkdir(filepath.Join(pluginDir, requiredNoctaliaPlugins[0]), 0755); err != nil {
		t.Fatal(err)
	}

	err := installNoctalia(dotfiles, home, time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC))
	if err == nil {
		t.Fatal("esperava erro para plugins ausentes")
	}
	for _, name := range requiredNoctaliaPlugins[1:] {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("erro não menciona %s: %v", name, err)
		}
	}
	stateDir := filepath.Join(home, ".local", "state", "noctalia")
	for _, name := range []string{"settings.toml", "state.toml"} {
		if _, err := os.Lstat(filepath.Join(stateDir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s foi alterado sem todos os plugins", name)
		}
	}
}

func TestInstallNoctaliaCopiesFilesAndPreservesState(t *testing.T) {
	dotfiles, home, pluginDir := prepareNoctaliaTest(t)
	installNoctaliaTestPlugins(t, pluginDir)
	stateDir := filepath.Join(home, ".local", "state", "noctalia")
	for _, name := range []string{"settings.toml", "state.toml"} {
		if err := os.WriteFile(filepath.Join(stateDir, name), []byte("old "+name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	if err := installNoctalia(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"settings.toml", "state.toml"} {
		destination := filepath.Join(stateDir, name)
		info, err := os.Lstat(destination)
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("%s não é arquivo regular: %v", name, info.Mode())
		}
		content, err := os.ReadFile(destination)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "new "+name {
			t.Fatalf("cópia inesperada para %s: %q", name, content)
		}
		backup := destination + ".backup-20250102-030405"
		backupContent, err := os.ReadFile(backup)
		if err != nil {
			t.Fatal(err)
		}
		if string(backupContent) != "old "+name {
			t.Fatalf("backup inesperado para %s: %q", name, backupContent)
		}
	}
	for _, name := range requiredNoctaliaPlugins {
		if info, err := os.Stat(filepath.Join(pluginDir, name)); err != nil || !info.IsDir() {
			t.Fatalf("plugin %s foi removido: %v", name, err)
		}
	}
}

func TestInstallNoctaliaMigratesExistingSymlinks(t *testing.T) {
	dotfiles, home, pluginDir := prepareNoctaliaTest(t)
	installNoctaliaTestPlugins(t, pluginDir)
	stateDir := filepath.Join(home, ".local", "state", "noctalia")
	for _, name := range []string{"settings.toml", "state.toml"} {
		source := filepath.Join(dotfiles, "noctalia", name)
		if err := os.Symlink(source, filepath.Join(stateDir, name)); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	if err := installNoctalia(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"settings.toml", "state.toml"} {
		destination := filepath.Join(stateDir, name)
		info, err := os.Lstat(destination)
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("%s não foi migrado para arquivo regular", name)
		}
	}
}

func TestInstallNoctaliaIsIdempotent(t *testing.T) {
	dotfiles, home, pluginDir := prepareNoctaliaTest(t)
	installNoctaliaTestPlugins(t, pluginDir)
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	if err := installNoctalia(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	if err := installNoctalia(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(home, ".local", "state", "noctalia")
	if matches, err := filepath.Glob(filepath.Join(stateDir, "*.backup-*")); err != nil || len(matches) != 0 {
		t.Fatalf("backups inesperados: %v, err=%v", matches, err)
	}
}
