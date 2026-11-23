package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoctaliaRequiresPluginsAndBacksUpChangedFiles(t *testing.T) {
	source := filepath.Join("..", "cachyos", "noctalia")
	home := filepath.Join(t.TempDir(), `a"b`)
	directory := filepath.Join(t.TempDir(), "noctalia")
	location := backupLocation{path: filepath.Join(t.TempDir(), "Noctalia")}
	settings := filepath.Join(directory, "settings.toml")
	state := filepath.Join(directory, "state.toml")
	sdkTestFile(t, settings, "previous settings")
	sdkTestFile(t, state, "previous state")
	if _, err := configureNoctalia(source, directory, home, location); err == nil {
		t.Fatal("a configuração foi aplicada com plugins ausentes")
	}
	sdkTestContent(t, settings, "previous settings")
	sdkTestContent(t, state, "previous state")
	for _, plugin := range []string{"containers", "github-kanban", "zed-provider"} {
		if err := os.MkdirAll(filepath.Join(directory, "plugins", "materialized", "community", plugin), 0755); err != nil {
			t.Fatal(err)
		}
	}
	backup, err := configureNoctalia(source, directory, home, location)
	if err != nil || !pathInside(location.path, backup) {
		t.Fatalf("backup em %q: %v", backup, err)
	}
	sdkTestContent(t, filepath.Join(backup, "settings.toml"), "previous settings")
	sdkTestContent(t, filepath.Join(backup, "state.toml"), "previous state")
	data, err := os.ReadFile(settings)
	if err != nil || !strings.Contains(string(data), `a\"b/.wallpapers/desktop-4k.jpg`) ||
		strings.Contains(string(data), "noctaproton-vpn") || strings.Contains(string(data), "@DOTFILES_HOME@") {
		t.Fatalf("configuração do Noctalia inválida: %v", err)
	}
	if backup, err := configureNoctalia(source, directory, home, location); err != nil || backup != "" {
		t.Fatalf("reaplicação com backup %q: %v", backup, err)
	}
}
