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

func TestMainMenuHasAtMostFiveOptions(t *testing.T) {
	model := newModel(t.TempDir())
	items := model.items()
	if len(items) > 5 {
		t.Fatalf("menu principal possui %d opções", len(items))
	}
	want := []string{"Sistema", "Desenvolvimento", "Desktop", "Agentes de IA", "Sair"}
	if titles := itemTitles(items); !slices.Equal(titles, want) {
		t.Fatalf("opções inesperadas: %v", titles)
	}
}

func TestMainMenuCategoriesContainAllActions(t *testing.T) {
	model := newModel(t.TempDir())
	categories := map[screen][]string{
		screenSystem:      {"SSH", "Firewall", "Docker", "Drivers"},
		screenDevelopment: {"Git", "GPG", "Mise", "Ambiente JavaScript", "Ambiente Flutter", "Instalar IDEs", "Ferramentas de terminal"},
		screenDesktop:     {"Niri", "Noctalia", "Ghostty", "Obsidian", "Wallpapers", "Foto de perfil"},
		screenAIAgents:    {"Pi Agent"},
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
	want := []screen{screenSystem, screenDevelopment, screenDesktop, screenAIAgents}
	for i, destination := range want {
		item := model.items()[i]
		if item.goTo != destination || item.job != nil || item.quit {
			t.Fatalf("categoria %s não abre o submenu correto", item.title)
		}
	}
}
