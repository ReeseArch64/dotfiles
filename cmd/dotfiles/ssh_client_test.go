package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func prepareSSHClientTest(t *testing.T) (string, string) {
	t.Helper()
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(configPath(dotfiles, "ssh"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath(dotfiles, "ssh", "config"), []byte("Host github.com\n"), 0755); err != nil {
		t.Fatal(err)
	}
	return dotfiles, home
}

func installSSHClientTestIdentities(t *testing.T, home string) {
	t.Helper()
	for _, name := range requiredSSHIdentityFiles {
		if err := os.WriteFile(filepath.Join(home, ".ssh", name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInstallSSHClientConfigRequiresEveryIdentityFile(t *testing.T) {
	dotfiles, home := prepareSSHClientTest(t)

	err := installSSHClientConfig(dotfiles, home, time.Now())
	if err == nil {
		t.Fatal("esperava erro para identidades ausentes")
	}
	for _, name := range requiredSSHIdentityFiles {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("erro não menciona %s: %v", name, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(home, ".ssh", "config")); !os.IsNotExist(err) {
		t.Fatalf("configuração copiada sem as identidades: %v", err)
	}
}

func TestInstallSSHClientConfigCopiesWithSecurePermissions(t *testing.T) {
	dotfiles, home := prepareSSHClientTest(t)
	installSSHClientTestIdentities(t, home)

	if err := installSSHClientConfig(dotfiles, home, time.Now()); err != nil {
		t.Fatal(err)
	}
	sshDir := filepath.Join(home, ".ssh")
	if info, err := os.Stat(sshDir); err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("permissão inesperada para ~/.ssh: info=%v err=%v", info, err)
	}
	destination := filepath.Join(sshDir, "config")
	info, err := os.Lstat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
		t.Fatalf("configuração insegura: %v", info.Mode())
	}
	content, err := os.ReadFile(destination)
	if err != nil || string(content) != "Host github.com\n" {
		t.Fatalf("conteúdo inesperado: %q err=%v", content, err)
	}
}

func TestInstallSSHClientConfigPreservesExistingConfig(t *testing.T) {
	dotfiles, home := prepareSSHClientTest(t)
	installSSHClientTestIdentities(t, home)
	destination := filepath.Join(home, ".ssh", "config")
	if err := os.WriteFile(destination, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	if err := installSSHClientConfig(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	backup := destination + ".backup-20250102-030405"
	content, err := os.ReadFile(backup)
	if err != nil || string(content) != "old" {
		t.Fatalf("backup inesperado: %q err=%v", content, err)
	}
}

func TestInstallSSHClientConfigIsIdempotent(t *testing.T) {
	dotfiles, home := prepareSSHClientTest(t)
	installSSHClientTestIdentities(t, home)
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	if err := installSSHClientConfig(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	if err := installSSHClientConfig(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(home, ".ssh", "config.backup-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("backups inesperados: %v, err=%v", matches, err)
	}
}

func TestSSHMenuHasSingleConfigurationAction(t *testing.T) {
	model := newModel(t.TempDir())
	model.screen = screenSSH
	model.ssh.cfg = map[string]string{"port": "2222"}
	items := model.items()
	if titles := itemTitles(items); len(items) != 1 || titles[0] != "Configurar SSH" {
		t.Fatalf("ações SSH inesperadas: %v", titles)
	}
	labels := jobStepLabels(configureSSHJob(model.dotfiles, "2222", "192.168.1.0/24"))
	for _, want := range []string{
		"Validar chaves e copiar ~/.ssh/config",
		"Liberar 2222/tcp para 192.168.1.0/24",
		"Desabilitar sshd.socket",
		"Habilitar e iniciar sshd.service",
	} {
		if !slices.Contains(labels, want) {
			t.Fatalf("passo SSH %q ausente: %v", want, labels)
		}
	}
}

func TestFirewallMenuHasSingleLANConfigurationAction(t *testing.T) {
	model := newModel(t.TempDir())
	model.screen = screenFirewall
	model.fw = fwStatus{port: "2222", lan: "10.20.30.0/24"}
	items := model.items()
	if titles := itemTitles(items); len(items) != 1 || titles[0] != "Configurar Firewall" {
		t.Fatalf("ações do firewall inesperadas: %v", titles)
	}
	labels := jobStepLabels(items[0].job())
	for _, want := range []string{
		"Entrada: bloquear por padrão",
		"Saída: permitir por padrão",
		"Remover liberação global de 2222/tcp",
		"Liberar 2222/tcp para 10.20.30.0/24",
		"Habilitar ufw.service no boot",
		"Ativar UFW",
	} {
		if !slices.Contains(labels, want) {
			t.Fatalf("passo do firewall %q ausente: %v", want, labels)
		}
	}
}

func jobStepLabels(current job) []string {
	labels := make([]string, len(current.steps))
	for i, current := range current.steps {
		labels[i] = current.label
	}
	return labels
}
