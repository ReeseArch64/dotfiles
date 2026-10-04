package main

import (
	"fmt"
	"os"
	"path/filepath"
)

var obsoleteShellFiles = []string{".bashrc", ".bash_logout", ".bash_profile", ".zshrc"}

func installFishConfig(dotfiles, home string) error {
	source := configPath(dotfiles, "fish", "config.fish")
	destination := filepath.Join(home, ".config", "fish", "config.fish")
	legacyActivation := filepath.Join(home, ".config", "fish", "conf.d", "dotfiles-mise.fish")
	if err := copyFileAtomic(source, destination); err != nil {
		return err
	}
	paths := append([]string{legacyActivation}, obsoleteShellFiles...)
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			path = filepath.Join(home, path)
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remover %s: %w", path, err)
		}
	}
	return nil
}

func fishConfigStep(dotfiles, home string) step {
	return nativeStep("Copiar configuração para ~/.config/fish/config.fish", func() error {
		return installFishConfig(dotfiles, home)
	})
}
