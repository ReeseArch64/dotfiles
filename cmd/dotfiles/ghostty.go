package main

import (
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func installGhostty(setupDir, home string, now time.Time) error {
	return copyDirectoryWithBackup(
		configPath(setupDir, "ghostty"),
		filepath.Join(home, ".config", "ghostty"),
		now,
	)
}

func ghosttyJob(setupDir string) job {
	home, _ := os.UserHomeDir()
	destination := filepath.Join(home, ".config", "ghostty")
	steps := ensureYayPkgs("ghostty")
	steps = append(steps, nativeStep("Copiar configuração para ~/.config/ghostty", func() error {
		return installGhostty(setupDir, home, time.Now())
	}))
	return job{
		title: "Configurar Ghostty",
		steps: steps,
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render("Configuração copiada para " + destination)
		},
	}
}
