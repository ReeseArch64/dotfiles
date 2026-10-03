package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSandboxPackagesUseRequestedSources(t *testing.T) {
	if want := []string{"podman", "podman-compose", "distrobox"}; !slices.Equal(sandboxPacmanPackages, want) {
		t.Fatalf("pacotes Pacman inesperados: %v", sandboxPacmanPackages)
	}
	if sandboxAurPackage != "podman-tui-bin" {
		t.Fatalf("pacote AUR inesperado: %s", sandboxAurPackage)
	}
}

func TestConfigureDistroboxForPodmanCreatesConfig(t *testing.T) {
	home := t.TempDir()
	if err := configureDistroboxForPodman(home); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(home, ".config", "distrobox", "distrobox.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != distroboxPodmanSetting+"\n" {
		t.Fatalf("configuração inesperada: %q", content)
	}
}

func TestConfigureDistroboxForPodmanPreservesOtherSettings(t *testing.T) {
	home := t.TempDir()
	config := filepath.Join(home, ".config", "distrobox", "distrobox.conf")
	if err := os.MkdirAll(filepath.Dir(config), 0755); err != nil {
		t.Fatal(err)
	}
	original := "container_manager=\"docker\"\ncontainer_always_pull=\"1\"\ncontainer_manager=\"docker\"\n"
	if err := os.WriteFile(config, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}

	if err := configureDistroboxForPodman(home); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(config)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(content), "container_manager=") != 1 {
		t.Fatalf("configuração duplicada: %s", content)
	}
	for _, want := range []string{distroboxPodmanSetting, `container_always_pull="1"`} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("configuração %q ausente: %s", want, content)
		}
	}
}

func TestCreateSandboxStepUsesArchLinuxImage(t *testing.T) {
	current := createSandboxStep()
	want := []string{"distrobox", "create", "--name", "sandbox", "--image", "archlinux"}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando Distrobox inesperado: %v", got)
	}
}

func TestSandboxEnvironmentOrdersInstallConfigAndCreate(t *testing.T) {
	pacmanStep := nativeStep("Instalar pacotes Pacman", func() error { return nil })
	aurStep := nativeStep("Instalar pacote AUR", func() error { return nil })
	configured := sandboxEnvironmentJobFor(t.TempDir(), []step{pacmanStep}, []step{aurStep})
	want := []string{
		"Instalar pacotes Pacman",
		"Instalar pacote AUR",
		"Configurar Distrobox para usar Podman",
		"Criar Distrobox sandbox com Arch Linux",
	}
	if got := jobStepLabels(configured); !slices.Equal(got, want) {
		t.Fatalf("passos inesperados: %v", got)
	}
}

func TestDevelopmentMenuIncludesSeparateSandboxEnvironment(t *testing.T) {
	model := newModel(t.TempDir())
	model.screen = screenDevelopment
	for _, current := range model.items() {
		if current.title == "Ambiente Sandbox" {
			if current.job == nil {
				t.Fatal("Ambiente Sandbox não inicia um job")
			}
			return
		}
	}
	t.Fatal("Ambiente Sandbox ausente do menu Desenvolvimento")
}
