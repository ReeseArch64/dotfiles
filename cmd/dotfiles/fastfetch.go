package main

import "path/filepath"

func installFastfetch(dotfiles, home string) error {
	return copyFileAtomic(
		configPath(dotfiles, "fastfetch", "config.json"),
		filepath.Join(home, ".config", "fastfetch", "config.json"),
	)
}

func fastfetchConfigStep(dotfiles, home string) step {
	return nativeStep("Copiar configuração para ~/.config/fastfetch", func() error {
		return installFastfetch(dotfiles, home)
	})
}
