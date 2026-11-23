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
	device := identifyProfile("desktop")
	for _, name := range []string{"desktop-4k.jpg", "desktop-ultrawide.jpg", "dual-monitor.jpg"} {
		sdkTestFile(t, filepath.Join(source, name), name)
	}
	sdkTestFile(t, filepath.Join(source, "laptop.jpg"), "laptop")
	sdkTestFile(t, filepath.Join(target, "personal.jpg"), "original")
	sdkTestFile(t, filepath.Join(target, "laptop.jpg"), "old laptop")

	backup, err := installWallpapers(device, source, home, location)
	if err != nil || !pathInside(location.path, backup) {
		t.Fatalf("backup em %q: %v", backup, err)
	}
	sdkTestContent(t, filepath.Join(backup, "personal.jpg"), "original")
	sdkTestContent(t, filepath.Join(backup, "laptop.jpg"), "old laptop")
	for _, name := range []string{"desktop-4k.jpg", "desktop-ultrawide.jpg", "dual-monitor.jpg"} {
		sdkTestContent(t, filepath.Join(target, name), name)
	}
	for _, name := range []string{"personal.jpg", "laptop.jpg"} {
		if _, err := os.Lstat(filepath.Join(target, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("arquivo %s ainda presente ou inacessível: %v", name, err)
		}
	}
	if backup, err := installWallpapers(device, source, home, location); err != nil || backup != "" {
		t.Fatalf("reinstalação com backup em %q: %v", backup, err)
	}
	laptop := t.TempDir()
	if backup, err := installWallpapers(identifyProfile("laptop"), source, laptop, location); err != nil || backup != "" {
		t.Fatalf("instalação no laptop com backup em %q: %v", backup, err)
	}
	sdkTestContent(t, filepath.Join(laptop, ".wallpapers", "laptop.jpg"), "laptop")
}
