package main

import (
	"bytes"
	"errors"
	"os"
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
	source := filepath.Join("..", "cachyos", "pi", "agent")
	location := backupLocation{path: filepath.Join(home, "backups")}
	var calls []string
	verifyConfig := func() {
		t.Helper()
		for _, name := range []string{"settings.json", filepath.Join("themes", "noctalia.json")} {
			want, err := os.ReadFile(filepath.Join(source, name))
			if err != nil {
				t.Fatal(err)
			}
			sdkTestContent(t, filepath.Join(agent, name), string(want))
		}
	}
	console := sshConsole{home: home, output: &bytes.Buffer{}, commands: sshCommands{
		capture: func(_ string, args ...string) (string, error) {
			if !slices.Equal(args, []string{"pacman", "-Q", "mise"}) && !slices.Equal(args, []string{"pacman", "-Q", "git"}) {
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
		if err := configurePiAgent(console, source, agent, location); err != nil {
			t.Fatal(err)
		}
	}
	verifyConfig()
	prefix := "mise --cd / exec pi bun node -- env PI_CODING_AGENT_DIR=" + agent + " pi "
	if want := []string{prefix + "--version", prefix + "update --extensions", prefix + "--version", prefix + "update --extensions"}; !slices.Equal(calls, want) {
		t.Fatalf("comandos = %v, esperava %v", calls, want)
	}
	backups, err := filepath.Glob(filepath.Join(location.path, "pi-*"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backup gerado: %v, %v", backups, err)
	}
	sdkTestContent(t, filepath.Join(backups[0], "settings.json"), `{"theme":"anterior"}`)
	sdkTestContent(t, filepath.Join(backups[0], "noctalia.json"), `{"name":"tema anterior"}`)
	sdkTestContent(t, filepath.Join(agent, "auth.json"), "credencial de teste")
}

func TestConfigurePiAgentStopsWhenMiseFails(t *testing.T) {
	home := t.TempDir()
	agent := filepath.Join(home, ".pi", "agent")
	failure := errors.New("Mise indisponível")
	console := sshConsole{home: home, output: &bytes.Buffer{}, commands: sshCommands{
		capture:     func(string, ...string) (string, error) { return "instalado", nil },
		interactive: func(...string) error { return failure },
	}}
	source := filepath.Join("..", "cachyos", "pi", "agent")
	if err := configurePiAgent(console, source, agent, backupLocation{}); !errors.Is(err, failure) {
		t.Fatalf("erro = %v, esperava %v", err, failure)
	}
	if _, err := os.Stat(agent); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("configuração criada antes de verificar o Pi: %v", err)
	}
}
