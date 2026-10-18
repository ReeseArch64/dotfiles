package main

import (
	"path/filepath"
	"time"
)

func installBtop(setupDir, home string, now time.Time) error {
	return copyDirectoryWithBackup(
		configPath(setupDir, "btop"),
		filepath.Join(home, ".config", "btop"),
		now,
	)
}

func btopConfigStep(setupDir, home string) step {
	return nativeStep("Copiar configuração para ~/.config/btop", func() error {
		return installBtop(setupDir, home, time.Now())
	})
}
