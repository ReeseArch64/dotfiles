package main

import (
	"slices"
	"strings"
	"testing"
)

func itemTitles(items []item) []string {
	titles := make([]string, len(items))
	for i, item := range items {
		titles[i] = item.title
	}
	return titles
}

func TestExitMenuIdentifiesHostname(t *testing.T) {
	tests := []struct {
		hostname string
		subtitle string
	}{
		{"desktop", "CachyOS · Niri · Noctalia Shell · by ReeseArch64 · Desktop"},
		{"desktop-alan", "CachyOS · Niri · Noctalia Shell · by ReeseArch64 · Desktop"},
		{"DESKTOP-ALAN", "CachyOS · Niri · Noctalia Shell · by ReeseArch64 · Desktop"},
		{"laptop", "Sem suporte · by ReeseArch64"},
		{"laptop-alan", "Sem suporte · by ReeseArch64"},
		{"homeprod", "Sem suporte · by ReeseArch64"},
		{"homeprod-alan", "Sem suporte · by ReeseArch64"},
		{"homelab", "Sem suporte · by ReeseArch64"},
		{"homelab-alan", "Sem suporte · by ReeseArch64"},
		{"notebook", "Sem suporte · by ReeseArch64"},
		{"laptopish", "Sem suporte · by ReeseArch64"},
		{"desktop-laptop", "CachyOS · Niri · Noctalia Shell · by ReeseArch64 · Desktop"},
		{"", "Sem suporte · by ReeseArch64"},
	}
	for _, tt := range tests {
		t.Run(tt.hostname, func(t *testing.T) {
			m := newExitMenu(tt.hostname)
			m.width, m.height = 100, 30
			if !strings.Contains(m.View(), tt.subtitle+" · v"+version) {
				t.Fatalf("subtítulo incorreto: %q", m.subtitle())
			}
			if items := m.items(); len(items) != 1 || items[0].title != "Sair" || !items[0].quit {
				t.Fatalf("opções inesperadas: %v", itemTitles(items))
			}
		})
	}
}

func TestMainMenuOptions(t *testing.T) {
	model := newModel(t.TempDir())
	items := model.items()
	want := []string{"Desenvolvimento", "Agentes de IA", "Sair"}
	if titles := itemTitles(items); !slices.Equal(titles, want) {
		t.Fatalf("opções inesperadas: %v", titles)
	}
}

func TestMainMenuCategoriesContainAllActions(t *testing.T) {
	model := newModel(t.TempDir())
	categories := map[screen][]string{
		screenSystem:      nil,
		screenSSH:         nil,
		screenFirewall:    nil,
		screenDevelopment: {"Android CLI", "Proton VPN CLI"},
		screenDesktop:     nil,
		screenAIAgents:    {"Ollama"},
		screenBackup:      nil,
		screenCleanup:     nil,
		screenDocker:      nil,
		screenGit:         nil,
		screenGPG:         nil,
		screenPiAgent:     nil,
		screenNoctalia:    nil,
	}
	for category, want := range categories {
		model.screen = category
		if titles := itemTitles(model.items()); !slices.Equal(titles, want) {
			t.Fatalf("submenu %s inesperado: %v", screenTitles[category], titles)
		}
	}
}

func TestNotebookMenuContainsOnlySelectedActions(t *testing.T) {
	model := newModelForDevice(t.TempDir(), deviceNotebook)
	categories := map[screen][]string{
		screenSystem:      {"SSH", "Firewall", "Docker", "Aplicativos pré-instalados"},
		screenDevelopment: {"Git", "GPG", "Ambiente de Desenvolvimento", "Linear CLI", "Android CLI", "Ferramentas de terminal"},
		screenDesktop:     {"Niri", "Login automático", "Noctalia", "Mozilla Firefox", "Wallpapers", "Foto de perfil"},
		screenAIAgents:    {"Pi Agent"},
		screenCleanup:     {"Remover aplicativos"},
		screenBackup:      {"Ponto de Restauração", "Backup de Projetos Locais", "Backup das Chaves SSH", "Backup das Chaves GPG"},
	}
	for category, want := range categories {
		model.screen = category
		if titles := itemTitles(model.items()); !slices.Equal(titles, want) {
			t.Fatalf("submenu %s do Notebook inesperado: %v", screenTitles[category], titles)
		}
	}
}

func TestMainMenuCategoriesOpenSubmenus(t *testing.T) {
	model := newModel(t.TempDir())
	want := []screen{screenDevelopment, screenAIAgents}
	for i, destination := range want {
		item := model.items()[i]
		if item.goTo != destination || item.job != nil || item.quit {
			t.Fatalf("categoria %s não abre o submenu correto", item.title)
		}
	}
}
