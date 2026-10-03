package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func installFishConfig(dotfiles, home string) error {
	source := configPath(dotfiles, "fish", "config.fish")
	destination := filepath.Join(home, ".config", "fish", "config.fish")
	legacyActivation := filepath.Join(home, ".config", "fish", "conf.d", "dotfiles-mise.fish")
	if err := os.Remove(legacyActivation); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remover %s: %w", legacyActivation, err)
	}
	return copyFileAtomic(source, destination)
}

func fishConfigStep(dotfiles, home string) step {
	return nativeStep("Copiar configuração para ~/.config/fish/config.fish", func() error {
		return installFishConfig(dotfiles, home)
	})
}
