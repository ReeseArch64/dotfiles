package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPiAgentMenu(t *testing.T) {
	for _, hostname := range []string{"desktop", "laptop", "homelab", "homeprod"} {
		menu := newMenu(hostname)
		isWorkstation := menu.profile.sshKeyName() != ""
		if item := menu.items()[5]; item.goTo != screenAIAgents || item.disabled == isWorkstation {
			t.Fatalf("Agentes de IA em %s: %+v", hostname, item)
		}
		if !isWorkstation {
			continue
		}
		next, _ := menu.handleKey("6")
		group := next.(model)
		items := group.items()
		if hostname == "desktop" {
			if len(items) != 2 || items[1].disabled || items[1].title != "Configurar Ollama (AMD)" {
				t.Fatalf("ação do Ollama ausente: %v", items)
			}
			command, err := actionCommand(items[1].action, items[1].title)
			if err != nil || !slices.Equal(command.Args[1:], []string{"--packages", "setup-ollama"}) {
				t.Fatalf("ação do Ollama: %v, %v", command, err)
			}
		} else if len(items) != 1 {
			t.Fatalf("agentes de IA no Laptop = %v", items)
		}
		next, _ = group.handleKey("1")
		page := next.(model)
		if group.screen != screenAIAgents || page.screen != screenPiAgent {
			t.Fatalf("navegação para Pi Agent em %s", hostname)
		}
		item := page.items()[0]
		command, err := actionCommand(item.action, item.title)
		if item.disabled || err != nil || !slices.Equal(command.Args[1:], []string{"--setup-pi-agent"}) {
			t.Fatalf("ação do Pi Agent: %v, %v", command, err)
		}
	}
}

func TestConfigurePiAgentBacksUpLegacySettings(t *testing.T) {
	home := t.TempDir()
	agent := filepath.Join(t.TempDir(), "agent com espaço")
	settings, theme := filepath.Join(agent, "settings.json"), filepath.Join(agent, "themes", "noctalia.json")
	sdkTestFile(t, settings, `{"theme":"anterior"}`)
	sdkTestFile(t, theme, `{"name":"tema anterior"}`)
	sdkTestFile(t, filepath.Join(agent, "auth.json"), "credencial de teste")
	source := filepath.Join("..", "cachyos", "pi", "agent", "settings.json")
	location := backupLocation{path: filepath.Join(home, "backups")}
	var calls []string
	mise := filepath.Join(home, ".local", "bin", "mise")
	lookup := func(name string) (string, error) {
		if name == mise {
			return mise, nil
		}
		return "", exec.ErrNotFound
	}
	verifyConfig := func() {
		t.Helper()
		got, err := os.ReadFile(settings)
		if err != nil {
			t.Fatal(err)
		}
		var config struct {
			Theme    string   `json:"theme"`
			Packages []string `json:"packages"`
		}
		if err := json.Unmarshal(got, &config); err != nil || config.Theme != "anterior" || len(config.Packages) == 0 {
			t.Fatalf("configuração do Pi: %+v, %v", config, err)
		}
		sdkTestContent(t, theme, `{"name":"tema anterior"}`)
	}
	console := sshConsole{home: home, output: &bytes.Buffer{}, commands: sshCommands{
		capture: func(_ string, args ...string) (string, error) {
			if !slices.Equal(args, []string{"pacman", "-Q", "git"}) {
				t.Fatalf("consulta de pacote inesperada: %v", args)
			}
			return "instalado", nil
		},
		interactive: func(args ...string) error {
			calls = append(calls, strings.Join(args, " "))
			if slices.Contains(args, "update") {
				verifyConfig()
			}
			return nil
		},
	}}
	for range 2 {
		if err := configurePiAgent(console, source, agent, location, lookup); err != nil {
			t.Fatal(err)
		}
	}
	verifyConfig()
	prefix := mise + " --cd / exec pi bun node -- env PI_CODING_AGENT_DIR=" + agent + " pi "
	if want := []string{prefix + "--version", prefix + "update --extensions", prefix + "--version", prefix + "update --extensions"}; !slices.Equal(calls, want) {
		t.Fatalf("comandos = %v, esperava %v", calls, want)
	}
	backups, err := filepath.Glob(filepath.Join(location.path, "pi-*"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backup gerado: %v, %v", backups, err)
	}
	sdkTestContent(t, filepath.Join(backups[0], "settings.json"), `{"theme":"anterior"}`)
	if _, err := os.Stat(filepath.Join(backups[0], "noctalia.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup inesperado do tema: %v", err)
	}
	sdkTestContent(t, filepath.Join(agent, "auth.json"), "credencial de teste")
}

func TestPiAgentSettingsDropsThemeFromOldTemplate(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "legacy-settings.json")
	sdkTestFile(t, source, `{"theme":"noctalia","packages":["npm:example-extension"]}`)
	settings, err := piAgentSettings(source, filepath.Join(directory, "settings.json"))
	if err != nil || bytes.Contains(settings, []byte(`"theme"`)) || !bytes.Contains(settings, []byte(`"npm:example-extension"`)) {
		t.Fatalf("configuração para instalação nova: %s, %v", settings, err)
	}
}

func TestConfigurePiAgentStopsWhenMiseFails(t *testing.T) {
	home := t.TempDir()
	agent := filepath.Join(home, ".pi", "agent")
	failure := errors.New("Mise indisponível")
	console := sshConsole{home: home, output: &bytes.Buffer{}, commands: sshCommands{
		capture:     func(string, ...string) (string, error) { return "instalado", nil },
		interactive: func(...string) error { return failure },
	}}
	source := filepath.Join("..", "cachyos", "pi", "agent", "settings.json")
	lookup := func(string) (string, error) { return "/usr/bin/mise", nil }
	if err := configurePiAgent(console, source, agent, backupLocation{}, lookup); !errors.Is(err, failure) {
		t.Fatalf("erro = %v, esperava %v", err, failure)
	}
	if _, err := os.Stat(agent); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("configuração criada antes de verificar o Pi: %v", err)
	}
}
