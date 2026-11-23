package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStorageMigrationLinksFoldersAndBacksUpConflicts(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	location := backupLocation{path: filepath.Join(t.TempDir(), "Storage")}
	sdkTestFile(t, filepath.Join(home, "Pictures", "photo"), "home")
	sdkTestFile(t, filepath.Join(root, "Pictures", "photo"), "storage")
	sdkTestFile(t, filepath.Join(root, "Pictures", "unique"), "storage only")
	sdkTestFile(t, filepath.Join(home, ".trash", "existing"), "keep")
	run := func(args ...string) error {
		t.Fatalf("comando inesperado: %v", args)
		return nil
	}
	for range 2 {
		if err := migrateDirectories(context.Background(), home, root, location, storageDirectoryMappings, run); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"Documents", "Downloads", "Pictures", "Videos", "Music"} {
			if link, err := os.Readlink(filepath.Join(home, name)); err != nil || link != filepath.Join(root, name) {
				t.Fatalf("link de %s = %q: %v", name, link, err)
			}
		}
	}
	sdkTestContent(t, filepath.Join(root, "Pictures", "photo"), "home")
	sdkTestContent(t, filepath.Join(root, "Pictures", "unique"), "storage only")
	sdkTestContent(t, filepath.Join(home, ".trash", "existing"), "keep")
	backups, err := filepath.Glob(filepath.Join(location.path, "*", "*", "original", "photo"))
	if err != nil || len(backups) != 2 {
		t.Fatalf("backups: %v, %v", backups, err)
	}
}

func TestConfigureWorkspacesKeepsExistingProjects(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	existingHome := t.TempDir()
	existing := filepath.Join(existingHome, "Projects", "local")
	sdkTestFile(t, existing, "original")
	run := func(args ...string) error {
		t.Fatalf("comando inesperado: %v", args)
		return nil
	}
	if err := configureWorkspaces(existingHome, root, run); err == nil {
		t.Fatal("diretório Projects existente aceito")
	}
	sdkTestContent(t, existing, "original")
	for range 2 {
		if err := configureWorkspaces(home, root, run); err != nil {
			t.Fatal(err)
		}
	}
	if link, err := os.Readlink(filepath.Join(home, "Projects")); err != nil || link != root {
		t.Fatalf("link de Projects = %q: %v", link, err)
	}
}

func TestNiriValidatesBeforeBackingUpAndReplacing(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, ".config", "niri")
	source := filepath.Join("..", "cachyos", "niri")
	location := backupLocation{path: filepath.Join(t.TempDir(), "Niri")}
	sdkTestFile(t, filepath.Join(target, "config.kdl"), "original")
	invalid := errors.New("configuração inválida")
	if _, err := configureNiri(source, target, home, location, func(string) error { return invalid }); !errors.Is(err, invalid) {
		t.Fatalf("validação inválida: %v", err)
	}
	sdkTestContent(t, filepath.Join(target, "config.kdl"), "original")
	validate := func(candidate string) error {
		if candidate != filepath.Join(source, "config.kdl") {
			t.Fatalf("candidato inesperado: %s", candidate)
		}
		return nil
	}
	backup, err := configureNiri(source, target, home, location, validate)
	if err != nil || !pathInside(location.path, backup) {
		t.Fatalf("backup em %q: %v", backup, err)
	}
	sdkTestContent(t, filepath.Join(backup, "config.kdl"), "original")
	if !sameAssetTree(source, target) {
		t.Fatal("configuração instalada difere da configuração do Desktop")
	}
	if backup, err := configureNiri(source, target, home, location, validate); err != nil || backup != "" {
		t.Fatalf("reaplicação com backup em %q: %v", backup, err)
	}
}
