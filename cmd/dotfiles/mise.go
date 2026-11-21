package main

import (
	"path/filepath"
)

func installMiseConfig(setupDir, home string) error {
	return copyFileAtomic(configPath(setupDir, "mise", "mise.toml"), filepath.Join(home, ".config", "mise", "config.toml"))
}

func miseSetupSteps(setupDir, home string) []step {
	steps := ensureYayPkgs("mise")
	steps = append(steps,
		nativeStep("Copiar configuração para ~/.config/mise/config.toml", func() error {
			return installMiseConfig(setupDir, home)
		}),
		terminalStep("Confiar na configuração do mise", "mise", "trust", filepath.Join(home, ".config", "mise", "config.toml")),
	)
	return steps
}
