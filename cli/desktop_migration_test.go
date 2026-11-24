package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestStorageMigrationArchivesOnlyOldHomeContent(t *testing.T) {
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
		if err := migrateDirectories(context.Background(), home, root, location, storageDirectoryMappings, true, run, io.Discard); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"Documents", "Downloads", "Pictures", "Videos", "Music"} {
			if link, err := os.Readlink(filepath.Join(home, name)); err != nil || link != filepath.Join(root, name) {
				t.Fatalf("link de %s = %q: %v", name, link, err)
			}
		}
	}
	sdkTestContent(t, filepath.Join(root, "Pictures", "photo"), "storage")
	sdkTestContent(t, filepath.Join(root, "Pictures", "unique"), "storage only")
	sdkTestContent(t, filepath.Join(home, ".trash", "existing"), "keep")
	backups, err := filepath.Glob(filepath.Join(location.path, "*", "*", "original", "photo"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups: %v, %v", backups, err)
	}
	sdkTestContent(t, backups[0], "home")
}

func TestStorageMigrationSkipsBackupAndLeavesExistingTargetsIntact(t *testing.T) {
	home, root := t.TempDir(), t.TempDir()
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "Downloads")
	for _, path := range []string{filepath.Join(home, "Downloads"), destination} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	// Any attempt to treat this existing destination as a tree to migrate is an error.
	if err := syscall.Mkfifo(filepath.Join(destination, "keep-pipe"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "Documents"), filepath.Join(home, "Documents")); err != nil {
		t.Fatal(err)
	}
	location := backupLocation{path: filepath.Join(t.TempDir(), "unprepared-backups", "Storage")}
	for range 2 {
		if err := migrateDirectories(context.Background(), home, root, location, storageDirectoryMappings, true, func(args ...string) error {
			t.Fatalf("comando inesperado: %v", args)
			return nil
		}, io.Discard); err != nil {
			t.Fatal(err)
		}
	}
	if info, err := os.Stat(destination); err != nil || !os.SameFile(before, info) {
		t.Fatalf("diretório do storage substituído: %v", err)
	}
	if _, err := os.Lstat(filepath.Dir(location.path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a migração preparou backup para pastas vazias ou links: %v", err)
	}
	if link, err := os.Readlink(filepath.Join(home, "Downloads")); err != nil || link != destination {
		t.Fatalf("link Downloads: %q, %v", link, err)
	}
	if info, err := os.Stat(filepath.Join(home, "Documents")); err != nil || !info.IsDir() {
		t.Fatalf("link Documents deveria reaproveitar o novo destino: %v", err)
	}
}

func TestStorageMigrationRestoresHomeIfEmptyDirectoryReceivesData(t *testing.T) {
	home, root, backups := sdkTestRoots(t)
	local := filepath.Join(home, "Downloads")
	if err := os.Mkdir(local, 0700); err != nil {
		t.Fatal(err)
	}
	ops := systemSDKOps(context.Background(), backups)
	ops.archiveHomeOnly = true
	rename := ops.rename
	ops.rename = func(source, target string) error {
		if source == local {
			sdkTestFile(t, filepath.Join(local, "arrived"), "keep")
		}
		return rename(source, target)
	}
	if err := prepareDirectoryLinks(home, root, storageDirectoryMappings, ops); err == nil {
		t.Fatal("esperado erro após receber dado durante a migração")
	}
	sdkTestContent(t, filepath.Join(local, "arrived"), "keep")
	if _, err := os.Lstat(filepath.Join(root, "Downloads")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destino temporário do storage ainda existe: %v", err)
	}
}

func TestStorageMigrationRestoresHomeAfterCrossFilesystemCopyFailure(t *testing.T) {
	home, root, backups := sdkTestRoots(t)
	local := filepath.Join(home, "Downloads")
	sdkTestFile(t, filepath.Join(local, "file"), "original")
	ops := systemSDKOps(context.Background(), backups)
	ops.archiveHomeOnly = true
	rename := ops.rename
	ops.rename = func(source, target string) error {
		if source == local && pathInside(backups, target) {
			return syscall.EXDEV
		}
		return rename(source, target)
	}
	failed := errors.New("cópia interrompida")
	ops.copyTree = func(source, target string) error {
		if source == local || !pathInside(home, source) || !pathInside(backups, target) {
			t.Fatalf("cópia esperava pasta local temporária e backup central: %s → %s", source, target)
		}
		if _, err := os.Lstat(local); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("a origem antiga continuou em uso durante a cópia: %v", err)
		}
		sdkTestContent(t, filepath.Join(source, "file"), "original")
		sdkTestFile(t, filepath.Join(target, "partial"), "partial copy")
		return failed
	}
	if err := prepareDirectoryLinks(home, root, storageDirectoryMappings, ops); !errors.Is(err, failed) {
		t.Fatalf("erro esperado na cópia: %v", err)
	}
	sdkTestContent(t, filepath.Join(local, "file"), "original")
	if _, err := os.Stat(filepath.Join(root, "Downloads")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destino temporário do storage ainda existe: %v", err)
	}
}

func TestStorageMigrationKeepsNewDestinationDataOnRollback(t *testing.T) {
	home, root, backups := sdkTestRoots(t)
	destination := filepath.Join(root, "Downloads")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ops := systemSDKOps(ctx, backups)
	ops.archiveHomeOnly = true
	rename := ops.rename
	ops.rename = func(source, target string) error {
		err := rename(source, target)
		if err == nil && target == destination {
			sdkTestFile(t, filepath.Join(destination, "new"), "preserve")
			cancel()
		}
		return err
	}
	if err := prepareDirectoryLinks(home, root, storageDirectoryMappings, ops); !errors.Is(err, context.Canceled) {
		t.Fatalf("migração deveria reverter ao cancelar: %v", err)
	}
	sdkTestContent(t, filepath.Join(destination, "new"), "preserve")
	if _, err := os.Lstat(filepath.Join(home, "Downloads")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("link ficou após reverter: %v", err)
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
