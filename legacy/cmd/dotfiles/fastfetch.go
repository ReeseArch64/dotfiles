package main

import "path/filepath"

func installFastfetch(setupDir, home string) error {
	return copyFileAtomic(
		configPath(setupDir, "fastfetch", "config.json"),
		filepath.Join(home, ".config", "fastfetch", "config.json"),
	)
}

func fastfetchConfigStep(setupDir, home string) step {
	return nativeStep("Copiar configuração para ~/.config/fastfetch", func() error {
		return installFastfetch(setupDir, home)
	})
}
