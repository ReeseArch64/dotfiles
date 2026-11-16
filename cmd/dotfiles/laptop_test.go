package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLaptopSubtitle(t *testing.T) {
	m := newModelForDevice("/repo", deviceLaptop)
	m.width, m.height = 120, 40
	view := m.View()
	if !strings.Contains(view, "Omarchy · Wayland · by ReeseArch64 · Laptop · v1.0.0") {
		t.Fatalf("subtítulo do laptop incorreto: %s", view)
	}
	for _, old := range []string{"CachyOS", "Noctalia Shell", "Laptop (Omarchy Linux)", "v0.15.0"} {
		if strings.Contains(view, old) {
			t.Fatalf("texto antigo na interface: %s", old)
		}
	}
}

func TestLaptopFirewallMenuAndJob(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range requiredSSHIdentityFiles(deviceNotebook) {
		if err := os.WriteFile(filepath.Join(home, ".ssh", name), []byte("ssh-ed25519 YWJj desktop\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"config", "authorized_keys"} {
		if err := os.WriteFile(filepath.Join(home, ".ssh", name), []byte("ssh-ed25519 YWJj desktop\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m := newModelForDevice("/repo", deviceLaptop)
	m.ssh = sshStatus{service: unitState{"active", "enabled"}, cfg: map[string]string{"port": "22"}}
	if got := itemTitles(m.items()); !slices.Equal(got, []string{"Firewall", "Sair"}) {
		t.Fatalf("menu inesperado: %v", got)
	}
	m.screen = screenFirewall
	m.fw = fwStatus{port: "22", lan: "192.168.1.0/24"}
	items := m.items()
	if got := itemTitles(items); !slices.Equal(got, []string{"Configurar Firewall"}) {
		t.Fatalf("menu firewall inesperado: %v", got)
	}
	labels := jobStepLabels(items[0].job())
	for _, want := range []string{"Verificar pré-requisito SSH", "Entrada: bloquear por padrão", "Saída: permitir por padrão", "Habilitar ufw.service no boot", "Ativar UFW", "Liberar 22/tcp para 192.168.1.0/24"} {
		if !slices.Contains(labels, want) {
			t.Fatalf("passo ausente: %s", want)
		}
	}
	if err := os.Remove(filepath.Join(home, ".ssh", "authorized_keys")); err != nil {
		t.Fatal(err)
	}
	// Um job criado antes da remoção também deve recusar a execução.
	if _, err := items[0].job().steps[0].run(); err == nil {
		t.Fatal("job não revalidou o pré-requisito SSH")
	}
	locked := m.items()[0]
	if !locked.disabled || locked.job != nil {
		t.Fatal("firewall deveria bloquear sem chave autorizada")
	}
}

func TestLaptopFirewallLockedWithoutSSH(t *testing.T) {
	m := newModelForDevice("/repo", deviceLaptop)
	if m.items()[0].title != "🔒 Firewall" {
		t.Fatal("cadeado ausente")
	}
	m.screen = screenFirewall
	it := m.items()[0]
	if !it.disabled || it.job != nil {
		t.Fatal("firewall disponível sem SSH")
	}
	next, cmd := m.selectItem(it)
	if cmd != nil || next.(model).screen != screenFirewall {
		t.Fatal("item bloqueado foi executado")
	}
}

func TestLaptopPlatformRequiresOmarchy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "os-release")
	for _, id := range []string{"omarchy", "arch", "cachyos", "ubuntu"} {
		if err := os.WriteFile(path, []byte("ID="+id+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		err := checkPlatformForDevice(deviceLaptop, path, func(string) (string, error) {
			t.Fatal("laptop não deve exigir ferramentas de desktop")
			return "", nil
		})
		if (err == nil) != (id == "omarchy") {
			t.Fatalf("ID=%s: %v", id, err)
		}
	}
}
