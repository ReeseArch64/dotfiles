package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func prepareWallpapersTest(t *testing.T) (string, string) {
	t.Helper()
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	source := filepath.Join(dotfiles, "wallpapers")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"android.jpg", "desktop.jpg", "iphone.jpg"} {
		if err := os.WriteFile(filepath.Join(source, name), []byte("new "+name), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dotfiles, home
}

func TestInstallWallpapersCopiesRegularDirectory(t *testing.T) {
	dotfiles, home := prepareWallpapersTest(t)
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	if err := installWallpapers(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(home, ".wallpapers")
	info, err := os.Lstat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("destino não é diretório regular: %v", info.Mode())
	}
	for _, name := range []string{"android.jpg", "desktop.jpg", "iphone.jpg"} {
		content, err := os.ReadFile(filepath.Join(destination, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "new "+name {
			t.Fatalf("conteúdo inesperado para %s: %q", name, content)
		}
	}
}

func TestInstallWallpapersPreservesExistingDirectory(t *testing.T) {
	dotfiles, home := prepareWallpapersTest(t)
	destination := filepath.Join(home, ".wallpapers")
	if err := os.MkdirAll(destination, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "old.jpg"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	if err := installWallpapers(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	backup := destination + ".backup-20250102-030405"
	if content, err := os.ReadFile(filepath.Join(backup, "old.jpg")); err != nil || string(content) != "old" {
		t.Fatalf("backup inesperado: content=%q err=%v", content, err)
	}
	if _, err := os.Stat(filepath.Join(destination, "desktop.jpg")); err != nil {
		t.Fatal(err)
	}
}

func TestInstallWallpapersIsIdempotent(t *testing.T) {
	dotfiles, home := prepareWallpapersTest(t)
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	if err := installWallpapers(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	if err := installWallpapers(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(home, ".wallpapers.backup-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("backups inesperados: %v, err=%v", matches, err)
	}
}
