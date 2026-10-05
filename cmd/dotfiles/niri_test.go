package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestInstallNiriCreatesRegularConfigDirectory(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	source := configPath(dotfiles, "niri")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "config.kdl"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := installNiri(dotfiles, home, time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(home, ".config", "niri")
	info, err := os.Lstat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("destino não é diretório regular: %v", info.Mode())
	}
	content, err := os.ReadFile(filepath.Join(destination, "config.kdl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "new" {
		t.Fatalf("conteúdo inesperado: %q", content)
	}
}

func TestInstallNiriPreservesExistingConfiguration(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	source := configPath(dotfiles, "niri")
	destination := filepath.Join(home, ".config", "niri")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "config.kdl"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destination, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "config.kdl"), []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	if err := installNiri(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	backup := destination + ".backup-20250102-030405"
	content, err := os.ReadFile(filepath.Join(backup, "config.kdl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "old" {
		t.Fatalf("backup inesperado: %q", content)
	}
	content, err = os.ReadFile(filepath.Join(destination, "config.kdl"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "new" {
		t.Fatalf("cópia inesperada: %q", content)
	}
	if info, err := os.Lstat(destination); err != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("destino não é diretório regular: info=%v err=%v", info, err)
	}
}

func TestInstallNiriMigratesExistingSymlink(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	source := configPath(dotfiles, "niri")
	destination := filepath.Join(home, ".config", "niri")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "config.kdl"), []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(source, destination); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	if err := installNiri(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("symlink não foi migrado: %v", info.Mode())
	}
	backup := destination + ".backup-20250102-030405"
	if info, err := os.Lstat(backup); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("backup do symlink ausente: info=%v err=%v", info, err)
	}
}

func TestInstallNiriCursorCopiesThemeToIcons(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	source := configPath(dotfiles, "cursor", "FrierenBLZ")
	if err := os.MkdirAll(filepath.Join(source, "cursors"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "index.theme"), []byte("[Icon Theme]\nName=FrierenBLZ\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "cursors", "default"), []byte("cursor"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := installNiriCursor(dotfiles, home, time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"index.theme":     "[Icon Theme]\nName=FrierenBLZ\n",
		"cursors/default": "cursor",
	} {
		content, err := os.ReadFile(filepath.Join(home, ".icons", "FrierenBLZ", filepath.FromSlash(name)))
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != want {
			t.Fatalf("conteúdo inesperado em %s: %q", name, content)
		}
	}
}

func TestNiriJobCopiesConfigurationAndCursor(t *testing.T) {
	configured := niriJobFor("/tmp/dotfiles", "/tmp/home", func() time.Time { return time.Time{} })
	want := []string{
		"Copiar configuração para ~/.config/niri",
		"Copiar cursor FrierenBLZ para ~/.icons",
	}
	if labels := jobStepLabels(configured); !slices.Equal(labels, want) {
		t.Fatalf("passos inesperados: %v", labels)
	}
}

func TestInstallNiriIsIdempotent(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	source := configPath(dotfiles, "niri")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	if err := installNiri(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	if err := installNiri(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	if matches, err := filepath.Glob(filepath.Join(home, ".config", "niri.backup-*")); err != nil || len(matches) != 0 {
		t.Fatalf("backups inesperados: %v, err=%v", matches, err)
	}
}
