package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestBackupBrowserCreatesArchive(t *testing.T) {
	home := t.TempDir()
	destination := t.TempDir()
	procRoot := t.TempDir()
	for _, name := range []string{".config/zen/profile", ".cache/zen/cache", ".local/share/keyrings/keys"} {
		path := filepath.Join(home, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := backupBrowserAt(context.Background(), home, destination, procRoot); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(destination, "zen-backup.tar")
	info, err := os.Stat(archive)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permissão do backup: %v", info.Mode())
	}
	listing, err := exec.Command("tar", "-tf", archive).Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".config/zen/profile", ".cache/zen/cache", ".local/share/keyrings/keys"} {
		if !strings.Contains(string(listing), name) {
			t.Fatalf("arquivo ausente do backup: %s", name)
		}
	}
}

func TestBackupBrowserRefusesOpenZen(t *testing.T) {
	procRoot := t.TempDir()
	pidPath := filepath.Join(procRoot, "123")
	if err := os.Mkdir(pidPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pidPath, "comm"), []byte("zen-browser\n"), 0600); err != nil {
		t.Fatal(err)
	}
	destination := t.TempDir()
	err := backupBrowserAt(context.Background(), t.TempDir(), destination, procRoot)
	if err == nil || !strings.Contains(err.Error(), "feche o Zen") {
		t.Fatalf("Zen aberto deveria impedir backup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "zen-backup.tar")); !os.IsNotExist(err) {
		t.Fatalf("arquivo de backup inesperado: %v", err)
	}
}

func TestBackupBrowserKeepsPreviousArchiveOnFailure(t *testing.T) {
	home := t.TempDir()
	destination := t.TempDir()
	archive := filepath.Join(destination, "zen-backup.tar")
	if err := os.WriteFile(archive, []byte("previous"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := backupBrowserAt(context.Background(), home, destination, t.TempDir()); err == nil {
		t.Fatal("backup sem fontes deveria falhar")
	}
	content, err := os.ReadFile(archive)
	if err != nil || string(content) != "previous" {
		t.Fatalf("backup anterior foi alterado: %q, %v", content, err)
	}
}

func TestBackupDestinationReadyRequiresOwnerAndPrivateMode(t *testing.T) {
	destination := t.TempDir()
	if err := os.Chmod(destination, 0700); err != nil {
		t.Fatal(err)
	}
	if !backupDestinationReady(destination, os.Getuid(), os.Getgid()) {
		t.Fatal("diretório privado do usuário deveria estar pronto")
	}
	if err := os.Chmod(destination, 0755); err != nil {
		t.Fatal(err)
	}
	if backupDestinationReady(destination, os.Getuid(), os.Getgid()) {
		t.Fatal("diretório público não deveria estar pronto")
	}
	if backupDestinationReady(filepath.Join(destination, "ausente"), os.Getuid(), os.Getgid()) {
		t.Fatal("diretório ausente não deveria estar pronto")
	}
}

func TestBackupDestinationSetupUsesSudoInstallArguments(t *testing.T) {
	want := []string{"install", "-d", "-m", "0700", "-o", "1000", "-g", "1001", "/mnt/backups"}
	if got := backupDestinationSetupArgs("/mnt/backups", 1000, 1001); !slices.Equal(got, want) {
		t.Fatalf("argumentos inesperados: %v", got)
	}
}

func TestBrowserBackupJobPreparesDestinationBeforeBackup(t *testing.T) {
	setup := nativeStep("Preparar /mnt/backups", func() error { return nil })
	configured := browserBackupJobFor([]step{setup}, func() error { return nil })
	if configured.title != "Backup do Navegador" {
		t.Fatalf("título inesperado: %s", configured.title)
	}
	want := []string{"Preparar /mnt/backups", "Criar zen-backup.tar"}
	if labels := jobStepLabels(configured); !slices.Equal(labels, want) {
		t.Fatalf("passos inesperados: %v", labels)
	}
}
