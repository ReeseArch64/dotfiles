package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

func sdkTestRoots(t *testing.T) (string, string, string) {
	t.Helper()
	base := t.TempDir()
	home, sdk, backups := filepath.Join(base, "home"), filepath.Join(base, "sdk"), filepath.Join(base, "backups")
	for _, dir := range []string{home, sdk, backups} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	return home, sdk, backups
}

func sdkTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func sdkTestContent(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil || string(got) != want {
		t.Fatalf("%s: conteúdo encontrado %q, esperado %q, erro %v", path, got, want, err)
	}
}

func sdkTestBackup(t *testing.T, root, origin string) string {
	t.Helper()
	var result string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.Name() != "origin.json" {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var metadata struct{ Origin string }
		if err := json.Unmarshal(data, &metadata); err != nil {
			return err
		}
		if metadata.Origin == origin {
			result = filepath.Join(filepath.Dir(path), "original")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Fatalf("backup de %s ausente", origin)
	}
	return result
}

func TestSDKMigrationApprovedDirectoriesAndBackups(t *testing.T) {
	home, root, backups := sdkTestRoots(t)
	pairs := map[string]string{
		".aur": "Aur", ".cache": "Cache", "Android": "Android", ".gradle": "Gradle",
		".bun": "Bun", ".pi": "Pi", ".codex": "Codex", ".claude": "Claude", ".npm": "Npm",
		".rustup": "Rustup", ".agent-browser": "AgentBrowser", ".cargo": "Cargo",
		".expo": "Expo", ".vite-plus": "VitePlus", ".docker": "Docker", ".dotnet": "Dotnet",
		".java": "Java", ".copilot": "Copilot", ".azure": "Azure", ".aws": "Aws",
		".gemini": "Gemini", ".railway": "Railway", ".tauri": "Tauri",
		"go": "Go", ".render": "Render", ".maple": "Maple",
	}
	excluded := []string{".local", ".local/share/Trash", ".trash", "Projects", "Documents", "Downloads", "Pictures", "Videos", "Music", ".vscode"}
	for _, name := range []string{"go", ".render", ".maple"} {
		sdkTestFile(t, filepath.Join(home, name, "fixture"), name)
	}
	for _, name := range excluded {
		sdkTestFile(t, filepath.Join(home, name, "fixture"), name)
	}
	sdkTestFile(t, filepath.Join(home, ".claude", "conflict"), "home")
	sdkTestFile(t, filepath.Join(root, "Claude", "conflict"), "previous target")
	sdkTestFile(t, filepath.Join(root, "Claude", "unique"), "target only")
	ops := systemSDKOps(context.Background(), backups)
	if err := prepareDirectoryLinks(home, root, sdkDirectoryMappings, ops); err != nil {
		t.Fatal(err)
	}
	links := make(map[string]string)
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		target, err := os.Readlink(filepath.Join(home, entry.Name()))
		if err != nil || filepath.Dir(target) != root {
			t.Fatalf("link inválido: %s, %v", target, err)
		}
		links[entry.Name()] = filepath.Base(target)
	}
	if !reflect.DeepEqual(links, pairs) {
		t.Fatalf("links instalados: %v", links)
	}
	if link, err := os.Readlink(filepath.Join(home, miseSDKPath)); err != nil || link != filepath.Join(root, "Mise") {
		t.Fatalf("link do Mise: %q, %v", link, err)
	}
	for _, name := range excluded {
		sdkTestContent(t, filepath.Join(home, name, "fixture"), name)
	}
	for _, name := range []string{"go", ".render", ".maple"} {
		sdkTestContent(t, filepath.Join(root, pairs[name], "fixture"), name)
	}
	sdkTestContent(t, filepath.Join(root, "Claude", "conflict"), "home")
	sdkTestContent(t, filepath.Join(root, "Claude", "unique"), "target only")
	sdkTestContent(t, filepath.Join(sdkTestBackup(t, backups, filepath.Join(home, ".claude")), "conflict"), "home")
	sdkTestContent(t, filepath.Join(sdkTestBackup(t, backups, filepath.Join(root, "Claude")), "conflict"), "previous target")
	ops.copyTree = func(string, string) error { t.Fatal("reaplicação copiou dados"); return nil }
	if err := prepareDirectoryLinks(home, root, sdkDirectoryMappings, ops); err != nil {
		t.Fatal(err)
	}
}

func TestSDKMigrationMiseWithLegacyLocalLink(t *testing.T) {
	home, root, backups := sdkTestRoots(t)
	location := filepath.Join(home, ".local", "share", "mise")
	target := filepath.Join(root, "Mise")
	legacy := filepath.Join(root, "Local")
	sdkTestFile(t, filepath.Join(legacy, "share", "mise", "conflict"), "home")
	if err := os.Symlink(legacy, filepath.Join(home, ".local")); err != nil {
		t.Fatal(err)
	}
	sdkTestFile(t, filepath.Join(target, "conflict"), "previous target")
	sdkTestFile(t, filepath.Join(target, "unique"), "target only")
	sdkTestFile(t, filepath.Join(home, ".cache", "fixture"), "cache")
	sdkTestFile(t, filepath.Join(home, ".local", "share", "other", "fixture"), "other")
	ops := systemSDKOps(context.Background(), backups)
	if err := prepareDirectoryLinks(home, root, sdkDirectoryMappings, ops); err != nil {
		t.Fatal(err)
	}
	if link, err := os.Readlink(location); err != nil || link != target {
		t.Fatalf("link para Mise = %q, erro %v", link, err)
	}
	sdkTestContent(t, filepath.Join(target, "conflict"), "home")
	sdkTestContent(t, filepath.Join(target, "unique"), "target only")
	sdkTestContent(t, filepath.Join(home, ".cache", "fixture"), "cache")
	sdkTestContent(t, filepath.Join(home, ".local", "share", "other", "fixture"), "other")
	sdkTestContent(t, filepath.Join(sdkTestBackup(t, backups, target), "conflict"), "previous target")
	ops.copyTree = func(string, string) error { t.Fatal("reaplicação copiou dados"); return nil }
	if err := prepareDirectoryLinks(home, root, sdkDirectoryMappings, ops); err != nil {
		t.Fatal(err)
	}
}

func TestSDKMigrationHandlesMissingAndLinkedParents(t *testing.T) {
	for _, scenario := range []string{"novo", "falha ao criar link", "ancestral simbólico"} {
		t.Run(scenario, func(t *testing.T) {
			home, root, backups := sdkTestRoots(t)
			ops := systemSDKOps(context.Background(), backups)
			local := filepath.Join(home, ".local")
			if scenario == "ancestral simbólico" {
				if err := os.Symlink(t.TempDir(), local); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "falha ao criar link" {
				ops.symlink = func(target, link string) error {
					if target == filepath.Join(root, "Mise") {
						return errors.New("falha simulada")
					}
					return os.Symlink(target, link)
				}
			}
			err := prepareDirectoryLinks(home, root, sdkDirectoryMappings, ops)
			if scenario == "novo" {
				if err != nil {
					t.Fatal(err)
				}
				if link, err := os.Readlink(filepath.Join(local, "share", "mise")); err != nil || link != filepath.Join(root, "Mise") {
					t.Fatalf("link = %q, erro %v", link, err)
				}
				return
			}
			if err == nil {
				t.Fatal("migração deveria falhar")
			}
			if _, err := os.Lstat(filepath.Join(root, "Mise")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("destino criado após falha: %v", err)
			}
			if scenario == "falha ao criar link" {
				if _, err := os.Lstat(local); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("diretório temporário preservado após falha: %v", err)
				}
			}
		})
	}
}

func TestSDKMigrationDoesNotReplaceUnexpectedEntry(t *testing.T) {
	home, root, backups := sdkTestRoots(t)
	sdkTestFile(t, filepath.Join(home, ".aur", "file"), "original")
	ops := systemSDKOps(context.Background(), backups)
	rename := ops.rename
	conflict := filepath.Join(home, ".cache")
	ops.rename = func(source, target string) error {
		if target == conflict && strings.HasPrefix(filepath.Base(filepath.Dir(source)), ".sdk-link-") {
			sdkTestFile(t, conflict, "arrived during migration")
		}
		return rename(source, target)
	}
	if err := prepareDirectoryLinks(home, root, sdkDirectoryMappings, ops); !errors.Is(err, os.ErrExist) {
		t.Fatalf("conflito deveria abortar a migração: %v", err)
	}
	sdkTestContent(t, conflict, "arrived during migration")
	sdkTestContent(t, filepath.Join(home, ".aur", "file"), "original")
	if info, err := os.Lstat(filepath.Join(home, ".aur")); err != nil || !info.IsDir() {
		t.Fatal("diretório anterior não restaurado")
	}
}

func TestSDKMigrationRollsBackAcrossFilesystems(t *testing.T) {
	home, root, _ := sdkTestRoots(t)
	backups, err := os.MkdirTemp("/dev/shm", "dotfiles-sdk-test-")
	if err != nil {
		t.Skipf("segundo sistema de arquivos indisponível: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(backups) })
	var a, b syscall.Stat_t
	if syscall.Stat(home, &a) != nil || syscall.Stat(backups, &b) != nil || a.Dev == b.Dev {
		t.Skip("segundo sistema de arquivos indisponível")
	}
	sdkTestFile(t, filepath.Join(home, ".aur", "file"), "original")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ops := systemSDKOps(ctx, backups)
	rename := ops.rename
	ops.rename = func(source, target string) error {
		err := rename(source, target)
		if err == nil && target == filepath.Join(home, ".aur") {
			cancel()
		}
		return err
	}
	if err := prepareDirectoryLinks(home, root, sdkDirectoryMappings, ops); !errors.Is(err, context.Canceled) {
		t.Fatalf("migração deveria reverter ao cancelar: %v", err)
	}
	sdkTestContent(t, filepath.Join(home, ".aur", "file"), "original")
	sdkTestContent(t, filepath.Join(sdkTestBackup(t, backups, filepath.Join(home, ".aur")), "file"), "original")
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatalf("destino ficou parcialmente migrado: %v, %v", entries, err)
	}
}
