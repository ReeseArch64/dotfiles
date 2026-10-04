package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestDiscordJobRunsInstallationInRequiredOrder(t *testing.T) {
	pacmanStep := nativeStep("Instalar discord via Pacman", func() error { return nil })
	aurStep := nativeStep("Instalar betterdiscordctl via AUR", func() error { return nil })
	openStep := nativeStep("Abrir Discord e carregar seus módulos", func() error { return nil })
	configured := discordJobFor(t.TempDir(), t.TempDir(), []step{pacmanStep}, []step{aurStep}, openStep)

	want := []string{
		"Instalar discord via Pacman",
		"Instalar betterdiscordctl via AUR",
		"Abrir Discord e carregar seus módulos",
		"Criar link do módulo Discord",
		"Instalar BetterDiscord",
		"Substituir arquivos do BetterDiscord",
	}
	if got := jobStepLabels(configured); !slices.Equal(got, want) {
		t.Fatalf("passos inesperados: %v", got)
	}
	install := configured.steps[4]
	if got := commandTail(t, install, 2); !slices.Equal(got, []string{"betterdiscordctl", "install"}) {
		t.Fatalf("comando BetterDiscord inesperado: %v", got)
	}
}

func TestDiscordUsesPacmanAndAurPackages(t *testing.T) {
	if discordPackage != "discord" {
		t.Fatalf("pacote Pacman inesperado: %s", discordPackage)
	}
	if betterDiscordPackage != "betterdiscordctl" {
		t.Fatalf("pacote AUR inesperado: %s", betterDiscordPackage)
	}
}

func TestOpenDiscordWaitsForVersionedCoreDirectory(t *testing.T) {
	home := t.TempDir()
	bin := t.TempDir()
	source, _ := discordCorePaths(home)
	writeTestExecutable(t, bin, "discord", "mkdir -p '"+source+"'")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := openDiscord(home, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(source); err != nil || !info.IsDir() {
		t.Fatalf("módulo Discord ausente: info=%v err=%v", info, err)
	}
}

func TestLinkDiscordCoreCreatesRequestedAbsoluteLink(t *testing.T) {
	home := t.TempDir()
	source, destination := discordCorePaths(home)
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}

	if err := linkDiscordCore(home); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(destination)
	if err != nil {
		t.Fatal(err)
	}
	if target != source {
		t.Fatalf("destino inesperado: %s", target)
	}
	if err := linkDiscordCore(home); err != nil {
		t.Fatalf("segunda execução falhou: %v", err)
	}
}

func TestInstallBetterDiscordConfigOverwritesManagedFiles(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	source := configPath(dotfiles, "discord", "BetterDiscord")
	destination := filepath.Join(home, ".config", "BetterDiscord")
	if err := os.MkdirAll(filepath.Join(source, "data"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(destination, "data"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "data", "settings.json"), []byte("managed"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "data", "settings.json"), []byte("downloaded"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "downloaded.js"), []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := installBetterDiscordConfig(dotfiles, home); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(destination, "data", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "managed" {
		t.Fatalf("configuração não foi substituída: %q", content)
	}
	if _, err := os.Stat(filepath.Join(destination, "downloaded.js")); err != nil {
		t.Fatalf("arquivo baixado não gerenciado foi removido: %v", err)
	}
}

func TestDesktopMenuIncludesDiscordJob(t *testing.T) {
	model := newModel(t.TempDir())
	model.screen = screenDesktop
	for _, current := range model.items() {
		if current.title == "Discord" {
			if current.job == nil || current.job().title != "Configurar Discord" {
				t.Fatal("ação Discord não inicia o job esperado")
			}
			return
		}
	}
	t.Fatal("ação Discord ausente do menu Desktop")
}
