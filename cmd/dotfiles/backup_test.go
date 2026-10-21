package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestRestorePointPathIncludesDateAndTime(t *testing.T) {
	testBackupStore(t)
	at := time.Date(2026, time.March, 7, 14, 5, 9, 0, time.Local)
	want := at.UTC().Format("20060102T150405.000000000Z") + "-"
	if got := restorePointPath(at); !strings.HasPrefix(got, filepath.Join(localBackupRoot(), "snapshots")+"/") || !strings.HasPrefix(filepath.Base(filepath.Dir(got)), want) || got == restorePointPath(at) {
		t.Fatalf("caminho inesperado: %s", got)
	}
}

func TestRestorePointUsesStandardWritableBtrfsSnapshot(t *testing.T) {
	testBackupStore(t)
	destination := restorePointPath(time.Now())
	want := []string{"bash", "-c", restorePointScript, "dotfiles-restore-point", destination, localBackupRoot()}
	if got := restorePointSnapshotArgs(destination); !slices.Equal(got, want) {
		t.Fatalf("argumentos inesperados: %v", got)
	}
}

func TestRestorePointJobCreatesSnapshotAfterPreparingDirectory(t *testing.T) {
	testBackupStore(t)
	destination := restorePointPath(time.Now())
	steps := []step{
		nativeStep("Validar filesystem Btrfs", func() error { return nil }),
		nativeStep("Criar snapshot Btrfs", func() error { return nil }),
	}
	configured := restorePointJobFor(destination, steps)
	if configured.title != "Ponto de Restauração" {
		t.Fatalf("título inesperado: %s", configured.title)
	}
	want := []string{"Validar filesystem Btrfs", "Criar snapshot Btrfs"}
	if labels := jobStepLabels(configured); !slices.Equal(labels, want) {
		t.Fatalf("passos inesperados: %v", labels)
	}
	if result := configured.result(); !strings.Contains(result, destination) {
		t.Fatalf("resultado não informa o destino: %q", result)
	}
}

func TestBackupBrowserCreatesArchive(t *testing.T) {
	testBackupStore(t)
	home := t.TempDir()
	destination := testPrivateDirectory(t)
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
	archive := filepath.Join(testBackupAt(t, destination, "browser", home), "zen-backup.tar")
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
	testBackupStore(t)
	procRoot := t.TempDir()
	pidPath := filepath.Join(procRoot, "123")
	if err := os.Mkdir(pidPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pidPath, "comm"), []byte("zen-browser\n"), 0600); err != nil {
		t.Fatal(err)
	}
	destination := testPrivateDirectory(t)
	err := backupBrowserAt(context.Background(), t.TempDir(), destination, procRoot)
	if err == nil || !strings.Contains(err.Error(), "feche o Zen") {
		t.Fatalf("Zen aberto deveria impedir backup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, "zen-backup.tar")); !os.IsNotExist(err) {
		t.Fatalf("arquivo de backup inesperado: %v", err)
	}
}

func TestBackupBrowserKeepsPreviousArchiveOnFailure(t *testing.T) {
	testBackupStore(t)
	home := t.TempDir()
	destination := testPrivateDirectory(t)
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
	testBackupStore(t)
	destination := testPrivateDirectory(t)
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
	testBackupStore(t)
	want := []string{"install", "-d", "-m", "0700", "-o", "1000", "-g", "1001", "/mnt/backups"}
	if got := backupDestinationSetupArgs("/mnt/backups", 1000, 1001); !slices.Equal(got, want) {
		t.Fatalf("argumentos inesperados: %v", got)
	}
}

func TestBackupSSHKeysCopiesIdentityFiles(t *testing.T) {
	testBackupStore(t)
	sshDir := t.TempDir()
	destination := filepath.Join(t.TempDir(), "ssh")
	for _, name := range requiredSSHIdentityFiles {
		mode := os.FileMode(0600)
		if strings.HasSuffix(name, ".pub") {
			mode = 0644
		}
		if err := os.WriteFile(filepath.Join(sshDir, name), []byte(name), mode); err != nil {
			t.Fatal(err)
		}
	}
	if err := backupSSHKeys(sshDir, destination); err != nil {
		t.Fatal(err)
	}
	destination = testBackupAt(t, destination, "ssh", sshDir)
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0700 {
		t.Fatalf("permiss\u00e3o do destino: %v", info.Mode())
	}
	for _, name := range requiredSSHIdentityFiles {
		content, err := os.ReadFile(filepath.Join(destination, name))
		if err != nil || string(content) != name {
			t.Fatalf("chave ausente ou alterada: %s (%v)", name, err)
		}
	}
}

func TestBackupSSHKeysFailsWhenKeysMissing(t *testing.T) {
	testBackupStore(t)
	sshDir := t.TempDir()
	destination := filepath.Join(t.TempDir(), "ssh")
	if err := backupSSHKeys(sshDir, destination); err == nil || !strings.Contains(err.Error(), "chaves ausentes") {
		t.Fatalf("deveria falhar sem chaves: %v", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destino inesperado: %v", err)
	}
}

func TestSSHKeysBackupJobCopiesAfterSetup(t *testing.T) {
	testBackupStore(t)
	setup := nativeStep("Preparar /mnt/backups", func() error { return nil })
	configured := sshKeysBackupJobFor([]step{setup}, func() error { return nil })
	if configured.title != "Backup das Chaves SSH" {
		t.Fatalf("t\u00edtulo inesperado: %s", configured.title)
	}
	want := []string{"Preparar /mnt/backups", "Copiar chaves SSH"}
	if labels := jobStepLabels(configured); !slices.Equal(labels, want) {
		t.Fatalf("passos inesperados: %v", labels)
	}
}

func TestBackupGPGKeysExportsPublicAndPrivateKeys(t *testing.T) {
	testBackupStore(t)
	destination := filepath.Join(t.TempDir(), "gpg")
	exporter := func(_ context.Context, secret bool) ([]byte, error) {
		if secret {
			return []byte("-----BEGIN PGP PRIVATE KEY BLOCK-----\nprivate\n"), nil
		}
		return []byte("-----BEGIN PGP PUBLIC KEY BLOCK-----\npublic\n"), nil
	}
	if err := backupGPGKeys(context.Background(), destination, exporter); err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	destination = testBackupAt(t, destination, "gpg", filepath.Join(home, ".gnupg"))
	checks := []struct {
		name string
		mode os.FileMode
		text string
	}{
		{"minha_chave_publica.asc", 0644, "public"},
		{"minha_chave_privada.asc", 0600, "private"},
	}
	for _, check := range checks {
		path := filepath.Join(destination, check.name)
		content, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(content), check.text) {
			t.Fatalf("conteúdo inesperado em %s: %q (%v)", check.name, content, err)
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != check.mode {
			t.Fatalf("permissão inesperada em %s: %v (%v)", check.name, info.Mode(), err)
		}
	}
	info, err := os.Stat(destination)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("permissão do diretório: %v (%v)", info.Mode(), err)
	}
}

func TestBackupGPGKeysDoesNotWriteWhenExportFails(t *testing.T) {
	testBackupStore(t)
	destination := filepath.Join(t.TempDir(), "gpg")
	exporter := func(_ context.Context, secret bool) ([]byte, error) {
		if secret {
			return nil, errors.New("falha privada")
		}
		return []byte("public"), nil
	}
	if err := backupGPGKeys(context.Background(), destination, exporter); err == nil {
		t.Fatal("exportação deveria falhar")
	}
	if entries, err := os.ReadDir(destination); !os.IsNotExist(err) && (err != nil || len(entries) != 0) {
		t.Fatalf("arquivos parciais encontrados: %v (%v)", entries, err)
	}
}

func TestGPGBackupJobExportsAfterSetup(t *testing.T) {
	testBackupStore(t)
	setup := nativeStep("Preparar /mnt/backups", func() error { return nil })
	configured := gpgBackupJobFor([]step{setup}, func() error { return nil })
	if configured.title != "Backup das Chaves GPG" {
		t.Fatalf("título inesperado: %s", configured.title)
	}
	want := []string{"Preparar /mnt/backups", "Exportar chaves GPG"}
	if labels := jobStepLabels(configured); !slices.Equal(labels, want) {
		t.Fatalf("passos inesperados: %v", labels)
	}
}

func TestBrowserBackupJobPreparesDestinationBeforeBackup(t *testing.T) {
	testBackupStore(t)
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
