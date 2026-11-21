package main

import (
	"slices"
	"strings"
	"testing"
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
				want = []string{"SSH", "Firewall", "Sair"}
			}
			if items := menuTitles(m.items()); !slices.Equal(items, want) {
				t.Fatalf("opções = %v, esperado %v", items, want)
			}
			if tt.keyName != "" {
				submenu, _ := m.selectItem(m.items()[0])
				want := []string{"Configurar SSH", "Criar chave SSH", "Adicionar chave pública", "Visualizar chaves autorizadas", "Remover chave pública", "Voltar"}
				if got := menuTitles(submenu.(model).items()); !slices.Equal(got, want) {
					t.Fatalf("submenu SSH = %v, esperado %v", got, want)
				}
			}
		})
	}
}

func menuTitles(items []menuItem) []string {
	var titles []string
	for _, item := range items {
		titles = append(titles, item.title)
	}
	return titles
}
