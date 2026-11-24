package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type dependencyResult struct {
	request int
	block   string
}

func dependencyCheck(page screen) func() string {
	switch page {
	case screenNoctalia:
		return func() string {
			home, err := os.UserHomeDir()
			if err != nil {
				return "Não foi possível identificar a pasta pessoal"
			}
			if n := len(missingNoctaliaPlugins(noctaliaDirectory(home))); n != 0 {
				return fmt.Sprintf("Plugins pendentes: %d (veja Requisitos dos plugins)", n)
			}
			return ""
		}
	case screenPreinstalled:
		return func() string {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if _, err := sshCommandsWithContext(ctx).capture("", "pacman", "-Q", "kitty"); err != nil {
				return "Instale Kitty em Programas → Terminal"
			}
			return ""
		}
	case screenProtonPass:
		return func() string {
			home, err := os.UserHomeDir()
			if err != nil {
				return "Não foi possível identificar a pasta pessoal"
			}
			installer := packageInstaller{console: &sshConsole{home: home}, which: exec.LookPath}
			if err := installer.checkProtonPassFirefox(); err != nil {
				return "Instale e abra o Firefox pelo menos uma vez"
			}
			return ""
		}
	}
	if login, ok := miseLoginForScreen(page); ok {
		return func() string {
			home, err := os.UserHomeDir()
			if err != nil {
				return "Não foi possível identificar a pasta pessoal"
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			c := sshConsole{home: home, commands: sshCommandsWithContext(ctx)}
			if _, err := login.command(&c, exec.LookPath); err != nil {
				return "Instale " + login.argv[0] + " em Programação → Mise"
			}
			return ""
		}
	}
	return nil
}

func (m model) dependencyBlock(item menuItem) string {
	if m.profile.sshKeyName() == "" {
		return ""
	}
	if item.goTo == screenFirewall || item.action == "firewall" {
		if !m.status.sshReady {
			return "Requer SSH configurado"
		}
		return ""
	}
	if item.action == "setup-docker" {
		if !m.docker.loaded {
			return "Verificando requisitos…"
		}
		if !m.docker.firefox {
			return "Instale o Firefox em Navegador Web"
		}
		return ""
	}
	switch item.action {
	case "setup-noctalia", "remove-preinstalled", "proton-pass":
	default:
		if _, ok := miseLoginForScreen(m.screen); !ok || item.action == "" {
			return ""
		}
	}
	if m.dependency.request == 0 || m.dependency.request != m.statusRequest {
		return "Verificando requisitos…"
	}
	return m.dependency.block
}
