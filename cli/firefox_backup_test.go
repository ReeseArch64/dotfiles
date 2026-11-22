package main

import (
	"archive/tar"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func firefoxFixtureFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCompleteFirefoxBackup(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, ".config")
	root := filepath.Join(config, "mozilla", "firefox")
	external := filepath.Join(t.TempDir(), "perfil externo")
	files := map[string]string{
		filepath.Join(root, "profiles.ini"):                              "[Profile0]\nPath=first\nIsRelative=1\n[Profile1]\nIsRelative=0\nPath=" + external + "\n",
		filepath.Join(root, "first", "key4.db"):                          "NSS key",
		filepath.Join(root, "first", "logins.json"):                      "encrypted login fixture",
		filepath.Join(root, "first", "cert9.db"):                         "NSS cert",
		filepath.Join(root, "first", "cookies.sqlite"):                   "cookies",
		filepath.Join(root, "first", "sessionstore.jsonlz4"):             "open tabs",
		filepath.Join(root, "first", "storage", "default", "site"):       "site data",
		filepath.Join(root, "Profile Groups", "group.sqlite"):            "group data",
		filepath.Join(home, ".mozilla", "firefox", "another", "key4.db"): "legacy key",
		filepath.Join(external, "key4.db"):                               "external key",
	}
	for path, data := range files {
		firefoxFixtureFile(t, path, data)
	}
	firefoxFixtureFile(t, filepath.Join(home, ".local", "share", "keyrings", "private.keyring"), "unrelated desktop credentials")

	dest := filepath.Join(t.TempDir(), "Firefox")
	now := time.Date(2026, 11, 21, 14, 32, 15, 0, time.Local)
	calls := 0
	sudo := func(command ...string) error {
		calls++
		want := []string{"sudo", "install", "-d", "-m", "0700", "-o", strconv.Itoa(os.Getuid()), "-g", strconv.Itoa(os.Getgid()), "--", dest}
		if !slices.Equal(command, want) {
			t.Fatalf("comando de criação incorreto: %v", command)
		}
		return os.Mkdir(dest, 0700)
	}
	checkClosed := func() error { return nil }

	backup, err := backupFirefox(context.Background(), home, config, dest, now, checkClosed, sudo)
	if err != nil {
		t.Fatal(err)
	}
	if backup != filepath.Join(dest, "firefox-2026-11-21-14-32-15.tar") || calls != 1 {
		t.Fatalf("destino inesperado: %s, chamadas sudo: %d", backup, calls)
	}
	if info, err := os.Stat(backup); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("permissões incorretas no arquivo: %v, %v", info, err)
	}
	archive, err := os.Open(backup)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	reader := tar.NewReader(archive)
	packed := make(map[string]string)
	for {
		entry, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if entry.Typeflag == tar.TypeReg {
			data, err := io.ReadAll(reader)
			if err != nil {
				t.Fatal(err)
			}
			packed[entry.Name] = string(data)
		}
	}
	for path, data := range files {
		_, member := tarLocation(home, path)
		if packed[member] != data {
			t.Errorf("arquivo ausente ou incorreto no tar: %s", member)
		}
	}
	for name := range packed {
		if strings.Contains(name, "keyring") || strings.HasSuffix(name, "/.parentlock") {
			t.Errorf("arquivo fora do escopo do backup: %s", name)
		}
	}

	if _, err := backupFirefox(context.Background(), home, config, dest, now, checkClosed, sudo); !errors.Is(err, os.ErrExist) {
		t.Fatalf("a segunda execução deveria preservar o tar existente: %v", err)
	}
	entries, err := os.ReadDir(dest)
	if err != nil || len(entries) != 1 || calls != 1 {
		t.Fatalf("arquivos inesperados depois da segunda execução: %v, %v", entries, err)
	}
}

func TestFirefoxMustStayClosed(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, ".config")
	firefoxFixtureFile(t, filepath.Join(config, "mozilla", "firefox", "profile", "key4.db"), "fixture")
	dest := filepath.Join(t.TempDir(), "Firefox")
	var checks atomic.Int32
	sudo := func(...string) error {
		return os.Mkdir(dest, 0700)
	}

	_, err := backupFirefox(context.Background(), home, config, dest, time.Now(), func() error {
		return errFirefoxOpen
	}, sudo)
	if !errors.Is(err, errFirefoxOpen) {
		t.Fatalf("a abertura do Firefox deveria bloquear a ação: %v", err)
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destino criado com o Firefox aberto: %v", err)
	}

	_, err = backupFirefox(context.Background(), home, config, dest, time.Now(), func() error {
		if checks.Add(1) >= 3 {
			return errFirefoxOpen
		}
		return nil
	}, sudo)
	if !errors.Is(err, errFirefoxOpen) {
		t.Fatalf("a reabertura do Firefox deveria cancelar o backup: %v", err)
	}
	if entries, err := os.ReadDir(dest); err != nil || len(entries) != 0 {
		t.Fatalf("sobrou arquivo depois do cancelamento: %v, %v", entries, err)
	}
}
