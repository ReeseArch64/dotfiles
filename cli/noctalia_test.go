package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNoctaliaRequiresPluginsAndBacksUpChangedFiles(t *testing.T) {
	source := t.TempDir()
	sdkTestFile(t, filepath.Join(source, "settings.toml"), "[wallpaper.default]\npath = \"@DOTFILES_HOME@/.wallpapers/desktop-4k.jpg\"\n")
	sdkTestFile(t, filepath.Join(source, "state.toml"), "[wallpaper]\npath = \"@DOTFILES_HOME@/.wallpapers/desktop-4k.jpg\"\n")
	home := filepath.Join(t.TempDir(), `a"b`)
	directory := filepath.Join(t.TempDir(), "noctalia")
	location := backupLocation{path: filepath.Join(t.TempDir(), "Noctalia")}
	settings := filepath.Join(directory, "settings.toml")
	state := filepath.Join(directory, "state.toml")
	sdkTestFile(t, settings, "previous settings")
	sdkTestFile(t, state, "previous state")
	community := []string{
		"androidconnect", "github-activity", "godot-provider", "kuma-status", "kumacat", "pomodoro-heatmap",
		"process-manager", "proton-pass", "shipyard", "ufw", "zed-provider",
	}
	const missingPrefix = "baixe os plugins ausentes no Noctalia: "
	if _, err := configureNoctalia(source, directory, home, location); err == nil || err.Error() != missingPrefix+strings.Join(community, ", ")+", world_clock" {
		t.Fatalf("lista de plugins ausentes incorreta: %v", err)
	}
	sdkTestContent(t, settings, "previous settings")
	sdkTestContent(t, state, "previous state")
	plugins := filepath.Join(directory, "plugins", "materialized")
	for _, plugin := range append(community, "world_clock") {
		if err := os.MkdirAll(filepath.Join(plugins, "community", plugin), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := configureNoctalia(source, directory, home, location); err == nil || err.Error() != missingPrefix+"world_clock" {
		t.Fatalf("a configuração deve exigir o World Clock oficial: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(plugins, "official", "world_clock"), 0755); err != nil {
		t.Fatal(err)
	}
	backup, err := configureNoctalia(source, directory, home, location)
	if err != nil || !pathInside(location.path, backup) {
		t.Fatalf("backup em %q: %v", backup, err)
	}
	sdkTestContent(t, filepath.Join(backup, "settings.toml"), "previous settings")
	sdkTestContent(t, filepath.Join(backup, "state.toml"), "previous state")
	for _, path := range []string{settings, state} {
		data, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(data), `a\"b/.wallpapers/desktop-4k.jpg`) || strings.Contains(string(data), "@DOTFILES_HOME@") {
			t.Fatalf("configuração do Noctalia inválida em %s: %v", path, err)
		}
	}
	if backup, err := configureNoctalia(source, directory, home, location); err != nil || backup != "" {
		t.Fatalf("reaplicação com backup %q: %v", backup, err)
	}
}
