package main

import (
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func installObsidian(setupDir, home string, now time.Time) error {
	return copyDirectoryWithBackup(
		configPath(setupDir, "obsidian"),
		filepath.Join(home, ".obsidian"),
		now,
	)
}

func obsidianJob(setupDir string) job {
	home, _ := os.UserHomeDir()
	destination := filepath.Join(home, ".obsidian")
	steps := ensureYayPkgs("obsidian-bin")
	steps = append(steps, nativeStep("Copiar configuração para ~/.obsidian", func() error {
		return installObsidian(setupDir, home, time.Now())
	}))
	return job{
		title: "Configurar Obsidian",
		steps: steps,
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render("Configuração copiada para " + destination)
		},
	}
}
