package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testPrivateDirectory(t *testing.T) string {
	t.Helper()
	path := t.TempDir()
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func testBackupStore(t *testing.T) string {
	t.Helper()
	root := testPrivateDirectory(t)
	t.Setenv("DOTFILES_BACKUP_DIR", root)
	return root
}

func testBackupPaths(root, category, origin string) ([]string, error) {
	origin, err := filepath.Abs(origin)
	if err != nil {
		return nil, err
	}
	return filepath.Glob(filepath.Join(root, category, fmt.Sprintf("%x", sha256.Sum256([]byte(origin))), "*", "original"))
}

func testBackupFor(t *testing.T, origin string) string {
	t.Helper()
	return testBackupAt(t, localBackupRoot(), "files", origin)
}

func testBackupAt(t *testing.T, root, category, origin string) string {
	t.Helper()
	paths, err := testBackupPaths(root, category, origin)
	if err != nil || len(paths) != 1 {
		t.Fatalf("backups for %s: %v, %v", origin, paths, err)
	}
	if !strings.Contains(filepath.Base(filepath.Dir(paths[0])), "Z-") {
		t.Fatalf("timestamp is not UTC with unique suffix: %s", paths[0])
	}
	return paths[0]
}

func TestDirectoryBackupUsesCentralStore(t *testing.T) {
	root := testBackupStore(t)
	base := t.TempDir()
	source, destination := filepath.Join(base, "source"), filepath.Join(base, "destination")
	sdkTestWrite(t, filepath.Join(source, "file"), "new")
	sdkTestWrite(t, filepath.Join(destination, "file"), "old")
	if err := copyDirectoryWithBackup(source, destination, time.Now()); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) == 0 {
		t.Fatalf("central backup store is empty: %v", err)
	}
	matches, _ := filepath.Glob(destination + ".backup-*")
	if len(matches) > 0 {
		t.Fatalf("adjacent backups created: %v", matches)
	}
}
