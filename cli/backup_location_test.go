package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
)

func TestManagedBackupRepairsUserOwnedDirectories(t *testing.T) {
	base := filepath.Join(t.TempDir(), "backups")
	path := filepath.Join(base, "dotfiles", "Fish")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(base, 0500); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(base, "dotfiles", "outro")
	if err := os.Mkdir(unrelated, 0755); err != nil {
		t.Fatal(err)
	}
	location := backupLocation{base: base, path: path, sudo: func(args ...string) error {
		t.Fatalf("sudo inesperado: %v", args)
		return nil
	}}
	backup, err := location.saveFile("config-*", []byte("anterior"))
	if err != nil {
		t.Fatal(err)
	}
	for name, mode := range map[string]os.FileMode{base: 0700, path: 0700, unrelated: 0755, backup: 0600} {
		info, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != mode {
			t.Errorf("%s tem modo %04o, esperado %04o", name, got, mode)
		}
	}
	if err := location.prepare(); err != nil {
		t.Fatal(err)
	}
}

func TestManagedBackupUsesSudoWhenNeededAndRejectsSymlinks(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("o teste depende das permissões de usuário normal")
	}
	parent := t.TempDir()
	base := filepath.Join(parent, "backups")
	location := backupLocation{base: base, path: filepath.Join(base, "dotfiles", "Fastfetch")}
	if err := os.Chmod(parent, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0700) })
	denied := errors.New("sudo recusado")
	calls := 0
	location.sudo = func(args ...string) error {
		calls++
		want := []string{"sudo", "install", "-d", "-m", "0755", "-o", strconv.Itoa(os.Geteuid()), "-g", strconv.Itoa(os.Getegid()), "--", base}
		if !slices.Equal(args, want) {
			t.Fatalf("recebi %v, esperava %v", args, want)
		}
		return denied
	}
	if err := location.prepare(); !errors.Is(err, denied) || calls != 1 {
		t.Fatalf("erro %v, chamadas sudo %d", err, calls)
	}
	if err := os.Chmod(parent, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), base); err != nil {
		t.Fatal(err)
	}
	if err := location.prepare(); err == nil || calls != 1 {
		t.Fatalf("link aceito ou sudo chamado: erro %v, chamadas %d", err, calls)
	}
}
