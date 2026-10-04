package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/charmbracelet/lipgloss"
)

const (
	discordPackage       = "discord"
	betterDiscordPackage = "betterdiscordctl"
	discordAppVersion    = "app-1.0.160"
)

func discordCorePaths(home string) (string, string) {
	modules := filepath.Join(home, ".config", "discord", discordAppVersion, "modules")
	return filepath.Join(modules, "discord_desktop_core-2"), filepath.Join(modules, "discord_desktop_core-1")
}

func openDiscord(home string, timeout time.Duration) error {
	source, _ := discordCorePaths(home)
	command := exec.Command("discord")
	if err := command.Start(); err != nil {
		return fmt.Errorf("abrir Discord: %w", err)
	}
	_ = command.Process.Release()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(source); err == nil && info.IsDir() {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("Discord não criou %s em %s", source, timeout)
}

func linkDiscordCore(home string) error {
	source, destination := discordCorePaths(home)
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("verificar %s: %w", source, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s não é um diretório", source)
	}
	if target, err := os.Readlink(destination); err == nil {
		if target == source {
			return nil
		}
		return fmt.Errorf("link existente em %s aponta para %s", destination, target)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("destino já existe em %s", destination)
	}
	if err := os.Symlink(source, destination); err != nil {
		return fmt.Errorf("criar link %s: %w", destination, err)
	}
	return nil
}

func installBetterDiscordConfig(dotfiles, home string) error {
	source := configPath(dotfiles, "discord", "BetterDiscord")
	destination := filepath.Join(home, ".config", "BetterDiscord")
	if info, err := os.Stat(source); err != nil {
		return fmt.Errorf("ler %s: %w", source, err)
	} else if !info.IsDir() {
		return fmt.Errorf("%s não é um diretório", source)
	}
	return copyDirectory(source, destination)
}

func discordJobFor(dotfiles, home string, pacmanSteps, aurSteps []step, openStep step) job {
	steps := append([]step{}, pacmanSteps...)
	steps = append(steps, aurSteps...)
	steps = append(steps,
		openStep,
		nativeStep("Criar link do módulo Discord", func() error {
			return linkDiscordCore(home)
		}),
		terminalStep("Instalar BetterDiscord", "betterdiscordctl", "install"),
		nativeStep("Substituir arquivos do BetterDiscord", func() error {
			return installBetterDiscordConfig(dotfiles, home)
		}),
	)
	return job{
		title: "Configurar Discord",
		steps: steps,
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render("Discord e BetterDiscord configurados.")
		},
	}
}

func discordJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	openStep := nativeStep("Abrir Discord e carregar seus módulos", func() error {
		return openDiscord(home, 60*time.Second)
	})
	return discordJobFor(
		dotfiles,
		home,
		ensurePkgs(discordPackage),
		ensureShellyPkgs("aur", betterDiscordPackage),
		openStep,
	)
}
