package main

import (
	"errors"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"testing"
)

func TestDistroboxSandboxMenu(t *testing.T) {
	menu := newMenu("desktop")
	menu.screen = screenDocker
	sandbox := menu.items()[1]
	command, err := actionCommand(sandbox.action, sandbox.title)
	if sandbox.disabled || sandbox.title != "Ambiente Sandbox" || err != nil || !slices.Equal(command.Args[1:], []string{"--setup-distrobox"}) {
		t.Fatalf("ação de Distrobox incorreta: %v %v %v", sandbox, command, err)
	}
}

func TestConfigureDistroboxUsesDockerAndPreservesSettings(t *testing.T) {
	const original = "# keep\nexport container_manager=podman\ncontainer_always_pull=1\n"
	for _, tc := range []struct {
		name, containers string
		wantCreate       bool
		wantError        bool
	}{
		{name: "create", containers: "sandbox-other distrobox\n", wantCreate: true},
		{name: "reuse", containers: "sandbox distrobox\n"},
		{name: "collision", containers: "sandbox \n", wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, configHome, backupRoot := sdkTestRoots(t)
			config := filepath.Join(home, ".distroboxrc")
			xdgConfig := filepath.Join(configHome, "distrobox", "distrobox.conf")
			sdkTestFile(t, config, original)
			sdkTestFile(t, xdgConfig, original)
			var created bool
			console := sshConsole{output: io.Discard, commands: sshCommands{
				capture: func(_ string, args ...string) (string, error) {
					switch {
					case slices.Equal(args, []string{"pacman", "-Q", "docker"}), slices.Equal(args, []string{"pacman", "-Q", "distrobox"}):
						return "installed", nil
					case slices.Equal(args, []string{"docker", "info", "--format", "{{.ServerVersion}}"}):
						return "docker", nil
					case slices.Equal(args, []string{"docker", "container", "ls", "--all", "--filter", "name=sandbox", "--format", `{{.Names}} {{.Label "manager"}}`}):
						return tc.containers, nil
					default:
						t.Fatalf("comando inesperado: %v", args)
						return "", nil
					}
				},
				interactive: func(args ...string) error {
					want := []string{"env", "DBX_CONTAINER_MANAGER=docker", "distrobox", "create", "--yes", "--name", "sandbox", "--image", "docker.io/library/archlinux:latest"}
					if !slices.Equal(args, want) {
						t.Fatalf("comando inesperado: %v", args)
					}
					created = true
					return nil
				},
			}}
			err := configureDistrobox(&console, &user.User{Username: "test", HomeDir: home}, configHome, backupLocation{path: backupRoot})
			if (err != nil) != tc.wantError || created != tc.wantCreate {
				t.Fatalf("criado = %t, erro = %v", created, err)
			}
			sdkTestContent(t, xdgConfig, original)
			if tc.wantError {
				sdkTestContent(t, config, original)
				return
			}
			sdkTestContent(t, config, "# keep\ncontainer_manager=\"docker\"\ncontainer_always_pull=1\n")
			entries, err := os.ReadDir(backupRoot)
			if err != nil || len(entries) != 1 {
				t.Fatalf("backup de configuração: %v %v", entries, err)
			}
			sdkTestContent(t, filepath.Join(backupRoot, entries[0].Name(), ".distroboxrc"), original)
		})
	}
}

func TestSandboxDockerAccessRefreshesGroup(t *testing.T) {
	var ran [][]string
	console := sshConsole{output: io.Discard, commands: sshCommands{
		capture: func(_ string, args ...string) (string, error) {
			if slices.Equal(args, []string{"newgrp", "docker", "-c", "'docker' 'info' '--format' '{{.ServerVersion}}'"}) {
				return "docker", nil
			}
			return "", errors.New("sem acesso ao Docker")
		},
		interactive: func(args ...string) error {
			ran = append(ran, args)
			return nil
		},
	}}
	commands, err := sandboxDockerAccess(&console, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := commands.interactive("env", "DBX_CONTAINER_MANAGER=docker", "distrobox", "create"); err != nil {
		t.Fatal(err)
	}
	want := [][]string{
		{"sudo", "systemctl", "enable", "--now", "docker.service"},
		{"sudo", "usermod", "-aG", "docker", "--", "test"},
		{"newgrp", "docker", "-c", "'env' 'DBX_CONTAINER_MANAGER=docker' 'distrobox' 'create'"},
	}
	if !slices.EqualFunc(ran, want, slices.Equal[[]string, string]) {
		t.Fatalf("comandos executados: %v", ran)
	}
}
