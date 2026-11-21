package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestMenuSelectsSSHKeyByHostname(t *testing.T) {
	tests := []struct {
		hostname string
		keyName  string
		subtitle string
	}{
		{"desktop-alan", "id_desktop", "CachyOS · Niri · Noctalia Shell · by ReeseArch64 · Desktop"},
		{"LAPTOP-ALAN", "id_laptop", "CachyOS · Niri · Noctalia Shell · by ReeseArch64 · Laptop"},
		{"homeprod-alan", "", "Ubuntu · by ReeseArch64 · HomeProd"},
		{"homelab-alan", "", "Ubuntu · by ReeseArch64 · Homelab"},
		{"desktop-laptop", "", "Sem suporte · by ReeseArch64"},
		{"notebook", "", "Sem suporte · by ReeseArch64"},
	}
	for _, tt := range tests {
		t.Run(tt.hostname, func(t *testing.T) {
			m := newMenu(tt.hostname)
			m.width = 100
			if !strings.Contains(m.View(), tt.subtitle+" · v"+version) {
				t.Fatalf("subtítulo incorreto: %q", m.subtitle())
			}
			if got := m.profile.sshKeyName(); got != tt.keyName {
				t.Fatalf("nome da chave = %q, esperado %q", got, tt.keyName)
			}
			want := []string{"Sair"}
			if tt.keyName != "" {
				want = []string{"Sistema Operacional", "Sair"}
			}
			if items := menuTitles(m.items()); !slices.Equal(items, want) {
				t.Fatalf("opções = %v, esperado %v", items, want)
			}
			if tt.keyName != "" {
				system, _ := m.selectItem(m.items()[0])
				osMenu := system.(model)
				if got := menuTitles(osMenu.items()); !slices.Equal(got, []string{"SSH", "Firewall", "Mouse personalizado"}) {
					t.Fatalf("menu do sistema = %v", got)
				}
				submenu, _ := osMenu.selectItem(osMenu.items()[0])
				ssh := submenu.(model)
				want := []string{"Configurar SSH", "Criar chave SSH", "Chaves autorizadas"}
				if got := menuTitles(ssh.items()); !slices.Equal(got, want) {
					t.Fatalf("submenu SSH = %v, esperado %v", got, want)
				}
				keys, _ := ssh.selectItem(ssh.items()[2])
				want = []string{"Adicionar chave pública", "Visualizar chaves atuais", "Remover chave pública"}
				if got := menuTitles(keys.(model).items()); !slices.Equal(got, want) {
					t.Fatalf("submenu de chaves = %v, esperado %v", got, want)
				}
			}
		})
	}
}

func TestLegacyMenuCardsAndCursor(t *testing.T) {
	m := newMenu("laptop-alan")
	m.width, m.height = 100, 35
	m.status = networkStatus{loaded: true, sshInstalled: true, sshActive: true, sshEnabled: true, port: "22", password: "no", root: "no"}
	operatingSystem, _ := m.handleKey("1")
	m = operatingSystem.(model)
	view := ansi.Strip(m.View())
	for _, want := range []string{"Menu principal › Sistema Operacional", "▌ SSH", "▌ Configurar SSH e gerenciar chaves autorizadas", "Firewall 2", "Requer SSH configurado", "Mouse personalizado 3", "1-9 atalho"} {
		if !strings.Contains(view, want) {
			t.Fatalf("menu principal deve conter %q", want)
		}
	}
	blocked, cmd := m.handleKey("2")
	if blocked.(model).screen != screenSystem || cmd != nil {
		t.Fatal("o firewall deve ficar desabilitado antes de configurar SSH")
	}
	m.status.sshReady = true
	firewall, _ := m.handleKey("2")
	if view := ansi.Strip(firewall.(model).View()); !strings.Contains(view, "Sistema Operacional › Firewall") || !strings.Contains(view, "LAN") {
		t.Fatalf("cartão de firewall ausente: %s", view)
	}
	ssh, _ := m.handleKey("1")
	if view := ansi.Strip(ssh.(model).View()); !strings.Contains(view, "Sistema Operacional › SSH") || !strings.Contains(view, "rodando (sshd.service)") {
		t.Fatalf("cartão de SSH ausente: %s", view)
	}
	keys, _ := ssh.(model).handleKey("3")
	if view := ansi.Strip(keys.(model).View()); !strings.Contains(view, "Sistema Operacional › SSH ›") ||
		!strings.Contains(view, "Chaves autorizadas") {
		t.Fatalf("caminho do submenu de chaves ausente: %s", view)
	}
	job, cmd := keys.(model).handleKey("1")
	if cmd == nil || job.(model).screen != screenJob {
		t.Fatal("selecionar ação deve abrir uma tela de execução")
	}
	done, _ := job.Update(actionResult{})
	if !strings.Contains(ansi.Strip(done.View()), "Concluído") {
		t.Fatal("tela de execução não mostra resultado")
	}
	back, _ := done.(model).handleJobKey("q")
	back, _ = back.(model).handleKey("esc")
	if back.(model).screen != screenSSH || back.(model).cursor[screenSSH] != 2 {
		t.Fatal("retorno deve preservar a posição no menu SSH")
	}
}

func menuTitles(items []menuItem) []string {
	var titles []string
	for _, item := range items {
		titles = append(titles, item.title)
	}
	return titles
}
