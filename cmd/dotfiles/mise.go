package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func installMiseConfig(dotfiles, home string) error {
	source, err := filepath.Abs(configPath(dotfiles, "mise", "mise.toml"))
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

func miseSetupSteps(dotfiles string) []step {
	home, _ := os.UserHomeDir()
	steps := ensureShellyPkgs("standard", "mise")
	steps = append(steps,
		nativeStep("Symlink ~/.config/mise/config.toml", func() error {
			return installMiseConfig(dotfiles, home)
		}),
		terminalStep("Confiar na configuração do mise", "mise", "trust", configPath(dotfiles, "mise", "mise.toml")),
	)
	return steps
}
