package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallWallpapersPreservesPreviousDirectory(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(t.TempDir(), "wallpapers")
	target := filepath.Join(home, ".wallpapers")
	location := backupLocation{path: filepath.Join(t.TempDir(), "Wallpapers")}
	sdkTestFile(t, filepath.Join(source, "desktop.jpg"), "desktop")
	sdkTestFile(t, filepath.Join(source, "laptop.jpg"), "laptop")
	sdkTestFile(t, filepath.Join(target, "personal.jpg"), "original")

	backup, err := installWallpapers(source, home, location)
	if err != nil || !pathInside(location.path, backup) {
		t.Fatalf("backup em %q: %v", backup, err)
	}
	sdkTestContent(t, filepath.Join(backup, "personal.jpg"), "original")
	sdkTestContent(t, filepath.Join(target, "desktop.jpg"), "desktop")
	sdkTestContent(t, filepath.Join(target, "laptop.jpg"), "laptop")
	if _, err := os.Lstat(filepath.Join(target, "personal.jpg")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("arquivo antigo ainda presente ou inacessível: %v", err)
	}
	if backup, err := installWallpapers(source, home, location); err != nil || backup != "" {
		t.Fatalf("reinstalação com backup em %q: %v", backup, err)
	}
}
