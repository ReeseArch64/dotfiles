package main

import (
	"slices"
	"testing"
)

func itemTitles(items []item) []string {
	titles := make([]string, len(items))
	for i, item := range items {
		titles[i] = item.title
	}
	return titles
}

func TestMainMenuOptions(t *testing.T) {
	model := newModel(t.TempDir())
	items := model.items()
	want := []string{"Sistema", "Desenvolvimento", "Desktop", "Agentes de IA", "Backup", "Sair"}
	if titles := itemTitles(items); !slices.Equal(titles, want) {
		t.Fatalf("opções inesperadas: %v", titles)
	}
}

func TestMainMenuCategoriesContainAllActions(t *testing.T) {
	model := newModel(t.TempDir())
	categories := map[screen][]string{
		screenSystem:      {"SSH", "Firewall", "Docker", "Drivers", "Aplicativos pré-instalados"},
		screenDevelopment: {"Git", "GPG", "Ambiente de Desenvolvimento", "Ambiente Sandbox", "Instalar IDEs", "Linear CLI", "Ferramentas de terminal"},
		screenDesktop:     {"Niri", "Noctalia", "Ghostty", "Zen Browser", "Thunderbird", "Discord", "Obsidian", "Wallpapers", "Foto de perfil"},
		screenAIAgents:    {"Pi Agent", "Ollama"},
		screenBackup:      {"Backup do Navegador", "Backup de Projetos Locais"},
		screenCleanup:     {"Remover aplicativos"},
	}
	for category, want := range categories {
		model.screen = category
		if titles := itemTitles(model.items()); !slices.Equal(titles, want) {
			t.Fatalf("submenu %s inesperado: %v", screenTitles[category], titles)
		}
	}
}

func TestMainMenuCategoriesOpenSubmenus(t *testing.T) {
	model := newModel(t.TempDir())
	want := []screen{screenSystem, screenDevelopment, screenDesktop, screenAIAgents, screenBackup}
	for i, destination := range want {
		item := model.items()[i]
		if item.goTo != destination || item.job != nil || item.quit {
			t.Fatalf("categoria %s não abre o submenu correto", item.title)
		}
	}
}
