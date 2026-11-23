package main

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestDockerMenu(t *testing.T) {
	for _, hostname := range []string{"desktop", "laptop", "homelab", "homeprod"} {
		m := newMenu(hostname)
		m.screen = screenSystem
		for _, item := range m.items() {
			if item.goTo == screenDocker {
				t.Fatalf("Docker aparece no menu do sistema de %s", hostname)
			}
		}
		m.screen = screenProgramming
		if m.profile.sshKeyName() == "" {
			for _, item := range m.items() {
				if item.goTo == screenDocker {
					t.Fatalf("Docker aparece no menu de programação de %s", hostname)
				}
			}
			continue
		}
		next, refresh := m.handleKey("3")
		docker := next.(model)
		if docker.screen != screenDocker || refresh == nil {
			t.Fatal("atalho não abriu o menu Docker")
		}
		docker.docker = dockerStatus{loaded: true, missing: []string{"docker-machine"}}
		view := docker.View()
		if !strings.Contains(view, "docker-machine ausente") || lipgloss.Height(view) != docker.height {
			t.Fatalf("status Docker inválido: %s", view)
		}
		for i, flag := range []string{"--setup-docker", "--setup-distrobox"} {
			item := docker.items()[i]
			command, err := actionCommand(item.action, item.title)
			if item.disabled || err != nil || !slices.Equal(command.Args[1:], []string{flag}) {
				t.Fatalf("comando %s em %s: %v, %v", flag, hostname, item, err)
			}
		}
	}
}

func TestConfigureDockerInstallsMachineAndManagesLogin(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "sem credenciais", true: "com credenciais"}[existing], func(t *testing.T) {
			config := t.TempDir()
			t.Setenv("DOCKER_CONFIG", config)
			if existing {
				content := []byte(`{"auths":{"https://index.docker.io/v1/":{"auth":"fixture"}}}`)
				if err := os.WriteFile(filepath.Join(config, "config.json"), content, 0600); err != nil {
					t.Fatal(err)
				}
			}
			var calls []string
			c := sshConsole{
				home: t.TempDir(), input: bufio.NewReader(strings.NewReader("CONFIRMO\n")), output: io.Discard,
				commands: sshCommands{
					capture: func(_ string, argv ...string) (string, error) {
						return "", errors.New("pacote ausente")
					},
					interactive: func(argv ...string) error {
						calls = append(calls, strings.Join(argv, " "))
						return nil
					},
				},
			}
			lookup := func(name string) (string, error) {
				if existing || name != "firefox" {
					t.Fatalf("busca inesperada: %s", name)
				}
				return "/usr/bin/firefox", nil
			}
			location := backupLocation{path: filepath.Join(t.TempDir(), "Docker")}
			if err := configureDocker(&c, "user", "/cli/dotfiles", lookup, location); err != nil {
				t.Fatal(err)
			}
			want := []string{
				"sudo pacman -S --needed -- docker docker-compose docker-buildx docker-machine lazydocker util-linux xdg-utils",
				"sudo -- /cli/dotfiles --apply-docker",
				"sudo usermod -aG docker -- user",
				"env -u BROWSER xdg-settings set default-web-browser firefox.desktop",
			}
			if !existing {
				want = append(want, "newgrp docker -c docker login")
			}
			if !slices.Equal(calls, want) {
				t.Fatalf("comandos = %q, esperado %q", calls, want)
			}
		})
	}
}

func TestApplyDockerDaemonValidatesAndRestoresOnStartFailure(t *testing.T) {
	for _, failure := range []string{"", "dockerd", "backup", "systemctl start"} {
		t.Run("falha="+failure, func(t *testing.T) {
			dir := t.TempDir()
			target := filepath.Join(dir, "daemon.json")
			before := []byte("{\"debug\": true}\n")
			if err := os.WriteFile(target, before, 0640); err != nil {
				t.Fatal(err)
			}
			var enabled bool
			commands := sshCommands{capture: func(_ string, argv ...string) (string, error) {
				switch {
				case argv[0] == "dockerd":
					if string(mustReadDockerFile(t, target)) != string(before) ||
						string(mustReadDockerFile(t, argv[3])) != dockerDaemonConfig {
						t.Fatal("validação deve usar candidato antes da troca")
					}
					if failure == "dockerd" {
						return "", errors.New("configuração inválida")
					}
				case argv[1] == "start":
					if string(mustReadDockerFile(t, target)) != dockerDaemonConfig {
						t.Fatal("daemon deve iniciar com a configuração nova")
					}
					if failure == "systemctl start" {
						return "", errors.New("serviço falhou")
					}
				case argv[1] == "enable":
					enabled = true
				}
				return "", nil
			}}
			var output strings.Builder
			location := backupLocation{path: filepath.Join(t.TempDir(), "Docker")}
			save := func(data []byte) (string, error) {
				if failure == "backup" {
					return "", errors.New("backup indisponível")
				}
				return location.saveFile("daemon.json.backup-*", data)
			}
			err := applyDockerDaemon(dir, commands, &output, save)
			if failure != "" {
				if err == nil || enabled || string(mustReadDockerFile(t, target)) != string(before) {
					t.Fatalf("configuração anterior não foi preservada: %v", err)
				}
				return
			}
			if err != nil || !enabled || !strings.Contains(output.String(), "próximo reinício") {
				t.Fatalf("aplicação incompleta: %v, %s", err, &output)
			}
			matches, err := filepath.Glob(filepath.Join(location.path, "daemon.json.backup-*"))
			if err != nil || len(matches) != 1 || string(mustReadDockerFile(t, matches[0])) != string(before) {
				t.Fatalf("backup inválido: %v, %v", matches, err)
			}
		})
	}
}

func mustReadDockerFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
