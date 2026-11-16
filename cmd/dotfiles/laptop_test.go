package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLaptopFirewallMenuAndJob(t *testing.T) {
	m := newModelForDevice("/repo", deviceLaptop)
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
	for _, want := range []string{"Entrada: bloquear por padrão", "Saída: permitir por padrão", "Habilitar ufw.service no boot", "Ativar UFW"} {
		if !slices.Contains(labels, want) {
			t.Fatalf("passo ausente: %s", want)
		}
	}
	for _, label := range labels {
		if strings.HasPrefix(label, "Liberar ") {
			t.Fatalf("laptop não deve liberar SSH: %s", label)
		}
	}
}

func TestLaptopPlatformRequiresOnlyArch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "os-release")
	for _, id := range []string{"arch", "cachyos", "ubuntu"} {
		if err := os.WriteFile(path, []byte("ID="+id+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		err := checkPlatformForDevice(deviceLaptop, path, func(string) (string, error) {
			t.Fatal("laptop não deve exigir ferramentas de desktop")
			return "", nil
		})
		if (err == nil) != (id == "arch") {
			t.Fatalf("ID=%s: %v", id, err)
		}
	}
}
