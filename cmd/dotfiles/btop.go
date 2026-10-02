package main

import (
	"path/filepath"
	"time"
)

func installBtop(dotfiles, home string, now time.Time) error {
	return copyDirectoryWithBackup(
		configPath(dotfiles, "btop"),
		filepath.Join(home, ".config", "btop"),
		now,
	)
}

func btopConfigStep(dotfiles, home string) step {
	return nativeStep("Copiar configuração para ~/.config/btop", func() error {
		return installBtop(dotfiles, home, time.Now())
	})
}
