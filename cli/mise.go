package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
)

func installMiseConfig(dotfiles, home string) error {
	source, err := filepath.Abs(filepath.Join(dotfiles, "mise.toml"))
	if err != nil {
		return fmt.Errorf("resolver mise.toml: %w", err)
	}
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("ler %s: %w", source, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s não é um arquivo regular", source)
	}
	destination := filepath.Join(home, ".config", "mise", "config.toml")
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return fmt.Errorf("criar %s: %w", filepath.Dir(destination), err)
	}
	if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remover %s: %w", destination, err)
	}
	if err := os.Symlink(source, destination); err != nil {
		return fmt.Errorf("criar %s: %w", destination, err)
	}
	return nil
}

const miseFishActivation = "mise activate fish | source\n"

func installMiseFishActivation(home string) error {
	destination := filepath.Join(home, ".config", "fish", "conf.d", "dotfiles-mise.fish")
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return fmt.Errorf("criar %s: %w", filepath.Dir(destination), err)
	}
	if err := os.WriteFile(destination, []byte(miseFishActivation), 0644); err != nil {
		return fmt.Errorf("gravar %s: %w", destination, err)
	}
	return nil
}

func miseSetupSteps(dotfiles string) []step {
	home, _ := os.UserHomeDir()
	steps := ensureShellyPkgs("standard", "mise")
	steps = append(steps,
		nativeStep("Symlink ~/.config/mise/config.toml", func() error {
			return installMiseConfig(dotfiles, home)
		}),
		nativeStep("Ativar ferramentas do mise no Fish", func() error {
			return installMiseFishActivation(home)
		}),
	)
	return steps
}

func miseJob(dotfiles string) job {
	steps := miseSetupSteps(dotfiles)
	return job{
		title: "Configurar mise",
		steps: steps,
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render(
				"Configuração ativa. Abra um novo terminal para usar as ferramentas diretamente.")
		},
	}
}
