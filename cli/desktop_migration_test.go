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

func TestNiriAppliesOnlyManagedFilesAfterValidation(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, ".config", "niri")
	source := t.TempDir()
	if err := copySDKTree(context.Background(), filepath.Join("..", "cachyos", "niri"), source); err != nil {
		t.Fatal(err)
	}
	location := backupLocation{path: filepath.Join(t.TempDir(), "Niri")}
	for _, name := range []string{"config.kdl", "cfg/input.kdl", "cfg/display.kdl"} {
		sdkTestFile(t, filepath.Join(target, name), "local "+name)
		sdkTestFile(t, filepath.Join(source, name), "asset de uma versão anterior")
	}
	sdkTestFile(t, filepath.Join(target, "local.kdl"), "configuração adicional")
	sdkTestFile(t, filepath.Join(target, "cfg", "rules.kdl"), "regras anteriores")
	invalid := errors.New("configuração inválida")
	if _, err := configureNiri(source, target, home, location, func(string) error { return invalid }); !errors.Is(err, invalid) {
		t.Fatalf("validação inválida: %v", err)
	}
	sdkTestContent(t, filepath.Join(target, "cfg", "rules.kdl"), "regras anteriores")
	validate := func(candidate string) error {
		staging := filepath.Dir(candidate)
		if filepath.Dir(staging) != filepath.Dir(target) || staging == target {
			t.Fatalf("candidato inesperado: %s", candidate)
		}
		for _, name := range []string{"config.kdl", "cfg/input.kdl", "cfg/display.kdl"} {
			sdkTestContent(t, filepath.Join(staging, name), "local "+name)
		}
		rules, err := os.ReadFile(filepath.Join(source, "cfg", "rules.kdl"))
		if err != nil {
			return err
		}
		sdkTestContent(t, filepath.Join(staging, "cfg", "rules.kdl"), string(rules))
		return nil
	}
	backup, err := configureNiri(source, target, home, location, validate)
	if err != nil || !pathInside(location.path, backup) {
		t.Fatalf("backup em %q: %v", backup, err)
	}
	sdkTestContent(t, filepath.Join(backup, "rules.kdl"), "regras anteriores")
	for _, name := range []string{"config.kdl", "cfg/input.kdl", "cfg/display.kdl"} {
		sdkTestContent(t, filepath.Join(target, name), "local "+name)
	}
	sdkTestContent(t, filepath.Join(target, "local.kdl"), "configuração adicional")
	for _, name := range niriManagedFiles {
		data, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatal(err)
		}
		sdkTestContent(t, filepath.Join(target, name), string(data))
	}
	if backup, err := configureNiri(source, target, home, location, validate); err != nil || backup != "" {
		t.Fatalf("reaplicação com backup em %q: %v", backup, err)
	}
}
