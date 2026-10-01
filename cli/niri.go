package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func installNiri(dotfiles, home string, now time.Time) error {
	source, err := filepath.Abs(filepath.Join(dotfiles, "niri"))
	if err != nil {
		return fmt.Errorf("resolver configuração do Niri: %w", err)
	}
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("ler %s: %w", source, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s não é um diretório", source)
	}

	configDir := filepath.Join(home, ".config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("criar %s: %w", configDir, err)
	}
	destination := filepath.Join(configDir, "niri")
	if directoriesEqual(source, destination) {
		return nil
	}

	staging, err := os.MkdirTemp(configDir, ".niri-copy-*")
	if err != nil {
		return fmt.Errorf("criar diretório temporário: %w", err)
	}
	defer os.RemoveAll(staging)
	if err := os.Chmod(staging, info.Mode().Perm()); err != nil {
		return fmt.Errorf("ajustar permissões temporárias: %w", err)
	}
	if err := copyDirectory(source, staging); err != nil {
		return fmt.Errorf("copiar configuração do Niri: %w", err)
	}

	var backup string
	if _, err := os.Lstat(destination); err == nil {
		backup = destination + ".backup-" + now.Format("20060102-150405")
		if _, err := os.Lstat(backup); err == nil {
			return fmt.Errorf("backup já existe: %s", backup)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("verificar %s: %w", backup, err)
		}
		if err := os.Rename(destination, backup); err != nil {
			return fmt.Errorf("preservar configuração atual em %s: %w", backup, err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("verificar %s: %w", destination, err)
	}

	if err := os.Rename(staging, destination); err != nil {
		if backup != "" {
			_ = os.Rename(backup, destination)
		}
		return fmt.Errorf("ativar configuração em %s: %w", destination, err)
	}
	return nil
}

func niriJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	destination := filepath.Join(home, ".config", "niri")
	steps := zenBrowserPrerequisiteSteps(home)
	steps = append(steps, nativeStep("Copiar configuração para ~/.config/niri", func() error {
		return installNiri(dotfiles, home, time.Now())
	}))
	return job{
		title: "Configurar Niri",
		steps: steps,
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render("Configuração copiada para " + destination)
		},
	}
}
