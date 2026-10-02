package main

import (
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func installGhostty(dotfiles, home string, now time.Time) error {
	return copyDirectoryWithBackup(
		configPath(dotfiles, "ghostty"),
		filepath.Join(home, ".config", "ghostty"),
		now,
	)
}

func ghosttyJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	destination := filepath.Join(home, ".config", "ghostty")
	steps := ensureShellyPkgs("standard", "ghostty")
	steps = append(steps, nativeStep("Copiar configuração para ~/.config/ghostty", func() error {
		return installGhostty(dotfiles, home, time.Now())
	}))
	return job{
		title: "Configurar Ghostty",
		steps: steps,
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render("Configuração copiada para " + destination)
		},
	}
}
