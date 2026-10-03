package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCleanupJobRequiresGhosttyBeforeRemovingAlacritty(t *testing.T) {
	configured := cleanupJobFor(t.TempDir(), func(...string) []string { return []string{"ghostty"} })
	if _, err := configured.steps[0].run(); err == nil || !strings.Contains(err.Error(), "Ghostty") {
		t.Fatalf("validação inesperada: %v", err)
	}

	configured = cleanupJobFor(t.TempDir(), func(...string) []string { return nil })
	if _, err := configured.steps[0].run(); err != nil {
		t.Fatalf("Ghostty instalado foi rejeitado: %v", err)
	}
}

func TestCleanupJobRefreshesPacmanBeforeRemoval(t *testing.T) {
	configured := cleanupJobFor(t.TempDir(), func(...string) []string { return nil })
	if got := jobStepLabels(configured); !slices.Equal(got, []string{
		"Verificar instalação do Ghostty",
		"Atualizar bases do Pacman",
		"Desinstalar firefox, alacritty, meld, micro",
		"Remover dados dos aplicativos",
	}) {
		t.Fatalf("passos inesperados: %v", got)
	}
	if got := commandTail(t, configured.steps[1], 3); !slices.Equal(got, []string{"sudo", "pacman", "-Syy"}) {
		t.Fatalf("sincronização inesperada: %v", got)
	}
	want := append([]string{"bash", "-c", removePreinstalledPackagesScript, "dotfiles-cleanup"}, preinstalledPackages...)
	if got := commandTail(t, configured.steps[2], len(want)); !slices.Equal(got, want) {
		t.Fatalf("remoção inesperada: %v", got)
	}
}

func TestRemovePreinstalledPackagesSkipsAbsentPackages(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "commands.log")
	writeTestExecutable(t, bin, "pacman", `[ "$1" = "-Q" ] && { [ "$2" = "alacritty" ] || [ "$2" = "micro" ]; }`)
	writeTestExecutable(t, bin, "sudo", `printf '%s\n' "$*" > "$COMMAND_LOG"`)
	t.Setenv("COMMAND_LOG", logPath)
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	configured := cleanupJobFor(t.TempDir(), func(...string) []string { return nil })
	if err := configured.steps[2].cmd().Run(); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.TrimSpace(string(content)), "pacman -Rns alacritty micro"; got != want {
		t.Fatalf("comando inesperado: %q", got)
	}
}

func TestRemovePreinstalledApplicationDirectories(t *testing.T) {
	home := t.TempDir()
	for _, path := range preinstalledApplicationDirectories(home) {
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "data"), []byte("old"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	keep := filepath.Join(home, ".config", "ghostty")
	if err := os.MkdirAll(keep, 0755); err != nil {
		t.Fatal(err)
	}

	if err := removePreinstalledApplicationDirectories(home); err != nil {
		t.Fatal(err)
	}
	for _, path := range preinstalledApplicationDirectories(home) {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("diretório não removido: %s", path)
		}
	}
	if info, err := os.Stat(keep); err != nil || !info.IsDir() {
		t.Fatalf("configuração do Ghostty foi alterada: info=%v err=%v", info, err)
	}
}
