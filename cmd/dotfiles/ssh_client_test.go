package main

import (
	"encoding/base64"
	"os"
	"os/exec"
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

func installSSHClientTestIdentities(t *testing.T, home string, device deviceProfile) {
	t.Helper()
	for _, name := range requiredSSHIdentityFiles(device) {
		content := name
		if strings.HasSuffix(name, ".pub") {
			content = "ssh-ed25519 " + base64.StdEncoding.EncodeToString([]byte(name)) + " " + name + "\n"
		}
		if err := os.WriteFile(filepath.Join(home, ".ssh", name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRequiredSSHIdentityFilesAreScopedByDevice(t *testing.T) {
	for device, want := range map[deviceProfile][]string{
		deviceDesktop: {
			"id_trabalho_desktop", "id_trabalho_desktop.pub", "id_pessoal_desktop", "id_pessoal_desktop.pub",
			"id_gitlab", "id_gitlab.pub", "id_github", "id_github.pub",
		},
		deviceNotebook: {
			"id_trabalho_notebook", "id_trabalho_notebook.pub", "id_trabalho_desktop.pub",
			"id_gitlab", "id_gitlab.pub", "id_github", "id_github.pub",
		},
		deviceServer: {"id_pessoal_desktop.pub"},
	} {
		if got := requiredSSHIdentityFiles(device); !slices.Equal(got, want) {
			t.Fatalf("%s: identidades SSH inesperadas: %v", device, got)
		}
	}
}

func TestIncomingSSHIdentitiesMatchDeviceRole(t *testing.T) {
	for device, want := range map[deviceProfile][]string{
		deviceDesktop:  nil,
		deviceNotebook: {"id_trabalho_desktop.pub"},
		deviceServer:   {"id_pessoal_desktop.pub"},
	} {
		if got, err := incomingSSHIdentities(device); err != nil || !slices.Equal(got, want) {
			t.Fatalf("%s: identidade=%q erro=%v", device, got, err)
		}
	}
	if _, err := incomingSSHIdentities(deviceMacBook); err == nil {
		t.Fatal("perfil sem configuração de entrada aceito")
	}
}

func TestSSHConfigsUseKeysFromTheirOriginDevice(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	desktop := readHomelabFile(t, configPath(setupDirectory(root, deviceDesktop), "ssh", "config"))
	for _, want := range []string{
		"IdentityFile ~/.ssh/id_trabalho_desktop",
		"IdentityFile ~/.ssh/id_pessoal_desktop",
	} {
		if !strings.Contains(desktop, want) {
			t.Fatalf("configuração desktop sem %s", want)
		}
	}
	if strings.Contains(desktop, "Host 192.168.1.104") {
		t.Fatal("desktop não deve configurar conexão SSH para si mesmo")
	}
	notebook := readHomelabFile(t, configPath(setupDirectory(root, deviceNotebook), "ssh", "config"))
	for _, forbidden := range []string{"id_pessoal_notebook", "Host 192.168.1.103", "Host 192.168.1.104", "Host 192.168.1.107"} {
		if strings.Contains(notebook, forbidden) {
			t.Fatalf("configuração notebook contém acesso não autorizado: %s", forbidden)
		}
	}
}

func TestDesktopAutomaticallyTrustsOnlyNewLANHostKeys(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	config := configPath(setupDirectory(root, deviceDesktop), "ssh", "config")
	for host, want := range map[string]string{
		"192.168.1.103": "accept-new",
		"192.168.1.107": "accept-new",
		"github.com":    "true",
	} {
		output, err := exec.Command("ssh", "-G", "-F", config, host).Output()
		if err != nil {
			t.Fatal(err)
		}
		setting := ""
		for _, line := range strings.Split(string(output), "\n") {
			if value, ok := strings.CutPrefix(line, "stricthostkeychecking "); ok {
				setting = value
				break
			}
		}
		if setting != want {
			t.Fatalf("%s: StrictHostKeyChecking=%q, esperado %q", host, setting, want)
		}
		if !strings.Contains(string(output), "identityagent ") || !strings.Contains(string(output), "/ssh-agent.socket") {
			t.Fatalf("%s: IdentityAgent não configurado", host)
		}
	}
}

func TestSSHConfigsAllowInteractiveTTY(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, device := range []deviceProfile{deviceDesktop, deviceNotebook} {
		config := configPath(setupDirectory(root, device), "ssh", "config")
		output, err := exec.Command("ssh", "-G", "-F", config, "github.com").Output()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(output), "requesttty auto\n") {
			t.Fatalf("%s: RequestTTY deve manter o padrão auto", device)
		}
	}
}

func TestAuthorizeSSHIdentitiesEnforcesDeviceSetAndIsIdempotent(t *testing.T) {
	testBackupStore(t)
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.Mkdir(sshDir, 0755); err != nil {
		t.Fatal(err)
	}
	var expected string
	for _, name := range []string{"id_trabalho_desktop.pub"} {
		line := "ssh-ed25519 " + base64.StdEncoding.EncodeToString([]byte(name)) + " " + name + "\n"
		expected += line
		if err := os.WriteFile(filepath.Join(sshDir, name), []byte(line), 0644); err != nil {
			t.Fatal(err)
		}
	}
	existing := "restrict ssh-ed25519 " + base64.StdEncoding.EncodeToString([]byte("existing-key")) + " existing@example\n"
	authorized := filepath.Join(sshDir, "authorized_keys")
	if err := os.WriteFile(authorized, []byte(existing), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	if err := authorizeSSHIdentities(home, []string{"id_trabalho_desktop.pub"}, now); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(authorized)
	if err != nil || string(first) != expected || strings.Contains(string(first), "existing-key") {
		t.Fatalf("authorized_keys inesperado: %q, %v", first, err)
	}
	backup := testBackupFor(t, authorized)
	if previous, err := os.ReadFile(backup); err != nil || string(previous) != existing {
		t.Fatalf("authorized_keys anterior não preservado: %q, %v", previous, err)
	}
	if err := authorizeSSHIdentities(home, []string{"id_trabalho_desktop.pub"}, now); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(authorized)
	if string(second) != string(first) {
		t.Fatalf("chave duplicada: %q", second)
	}
	if info, _ := os.Stat(sshDir); info.Mode().Perm() != 0700 {
		t.Fatalf("permissão de ~/.ssh: %v", info.Mode())
	}
	if info, _ := os.Stat(authorized); info.Mode().Perm() != 0600 {
		t.Fatalf("permissão de authorized_keys: %v", info.Mode())
	}
}

func TestDesktopRemovesAllIncomingSSHKeys(t *testing.T) {
	testBackupStore(t)
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.Mkdir(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	authorized := filepath.Join(sshDir, "authorized_keys")
	if err := os.WriteFile(authorized, []byte("ssh-ed25519 AAAA antiga\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := authorizeSSHIdentities(home, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	if content, err := os.ReadFile(authorized); err != nil || len(content) != 0 {
		t.Fatalf("desktop ainda aceita chaves: %q, %v", content, err)
	}
}

func TestAuthorizeSSHIdentitiesCreatesMissingEmptyFile(t *testing.T) {
	testBackupStore(t)
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.Mkdir(sshDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := authorizeSSHIdentities(home, nil, time.Now()); err != nil {
		t.Fatal(err)
	}
	authorized := filepath.Join(sshDir, "authorized_keys")
	content, err := os.ReadFile(authorized)
	if err != nil || len(content) != 0 {
		t.Fatalf("authorized_keys vazio não foi criado: %q, %v", content, err)
	}
	if info, err := os.Stat(authorized); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("permissão de authorized_keys: %v, %v", info, err)
	}
}

func TestNotebookCreatesMissingAuthorizedKeysWithDesktopKeys(t *testing.T) {
	testBackupStore(t)
	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.Mkdir(sshDir, 0755); err != nil {
		t.Fatal(err)
	}
	identities, err := incomingSSHIdentities(deviceNotebook)
	if err != nil {
		t.Fatal(err)
	}
	var expected string
	for _, name := range identities {
		line := "ssh-ed25519 " + base64.StdEncoding.EncodeToString([]byte(name)) + " " + name + "\n"
		expected += line
		if err := os.WriteFile(filepath.Join(sshDir, name), []byte(line), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := authorizeSSHIdentities(home, identities, time.Now()); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(sshDir, "authorized_keys"))
	if err != nil || string(content) != expected {
		t.Fatalf("authorized_keys do notebook: %q, %v", content, err)
	}
}

func TestInstallSSHClientConfigRequiresEveryIdentityFile(t *testing.T) {
	testBackupStore(t)
	dotfiles, home := prepareSSHClientTest(t)

	err := installSSHClientConfig(dotfiles, home, deviceDesktop, time.Now())
	if err == nil {
		t.Fatal("esperava erro para identidades ausentes")
	}
	for _, name := range requiredSSHIdentityFiles(deviceDesktop) {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("erro não menciona %s: %v", name, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(home, ".ssh", "config")); !os.IsNotExist(err) {
		t.Fatalf("configuração copiada sem as identidades: %v", err)
	}
}

func TestInstallSSHClientConfigCopiesWithSecurePermissions(t *testing.T) {
	testBackupStore(t)
	dotfiles, home := prepareSSHClientTest(t)
	installSSHClientTestIdentities(t, home, deviceDesktop)

	if err := installSSHClientConfig(dotfiles, home, deviceDesktop, time.Now()); err != nil {
		t.Fatal(err)
	}
	sshDir := filepath.Join(home, ".ssh")
	if info, err := os.Stat(sshDir); err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("permissão inesperada para ~/.ssh: info=%v err=%v", info, err)
	}
	knownHosts := filepath.Join(sshDir, "known_hosts")
	if info, err := os.Stat(knownHosts); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("known_hosts não preparado: info=%v err=%v", info, err)
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
	testBackupStore(t)
	dotfiles, home := prepareSSHClientTest(t)
	installSSHClientTestIdentities(t, home, deviceDesktop)
	destination := filepath.Join(home, ".ssh", "config")
	if err := os.WriteFile(destination, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	if err := installSSHClientConfig(dotfiles, home, deviceDesktop, now); err != nil {
		t.Fatal(err)
	}
	backup := testBackupFor(t, destination)
	content, err := os.ReadFile(backup)
	if err != nil || string(content) != "old" {
		t.Fatalf("backup inesperado: %q err=%v", content, err)
	}
}

func TestInstallSSHClientConfigIsIdempotent(t *testing.T) {
	testBackupStore(t)
	dotfiles, home := prepareSSHClientTest(t)
	installSSHClientTestIdentities(t, home, deviceDesktop)
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	if err := installSSHClientConfig(dotfiles, home, deviceDesktop, now); err != nil {
		t.Fatal(err)
	}
	if err := installSSHClientConfig(dotfiles, home, deviceDesktop, now); err != nil {
		t.Fatal(err)
	}
	matches, err := testBackupPaths(localBackupRoot(), "files", filepath.Join(home, ".ssh", "config"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("backups inesperados: %v, err=%v", matches, err)
	}
}

func TestSSHMenuHasSingleConfigurationAction(t *testing.T) {
	testBackupStore(t)
	model := newModel(t.TempDir())
	model.screen = screenSSH
	model.ssh.cfg = map[string]string{"port": "2222"}
	items := model.items()
	if titles := itemTitles(items); len(items) != 1 || titles[0] != "Configurar SSH" {
		t.Fatalf("ações SSH inesperadas: %v", titles)
	}
	labels := jobStepLabels(configureSSHJob(model.dotfiles, "2222", "192.168.1.0/24", deviceDesktop))
	for _, want := range []string{
		"Validar chaves e copiar ~/.ssh/config",
		"Autorizar chave para conexões SSH recebidas",
		"Habilitar ssh-agent do usuário",
		"Remover liberação LAN anterior de 2222/tcp",
		"Desabilitar sshd.socket",
		"Desabilitar sshd.service no desktop",
	} {
		if !slices.Contains(labels, want) {
			t.Fatalf("passo SSH %q ausente: %v", want, labels)
		}
	}
	for _, label := range labels {
		if strings.HasPrefix(label, "Liberar 2222/tcp") || label == "Habilitar e iniciar sshd.service" {
			t.Fatalf("desktop aceita SSH recebido: %v", labels)
		}
	}
}

func TestFirewallMenuHasSingleLANConfigurationAction(t *testing.T) {
	testBackupStore(t)
	model := newModel(t.TempDir())
	model.device = deviceNotebook
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
