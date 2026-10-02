package main

import (
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func installObsidian(dotfiles, home string, now time.Time) error {
	return copyDirectoryWithBackup(
		configPath(dotfiles, "obsidian"),
		filepath.Join(home, ".obsidian"),
		now,
	)
}

func obsidianJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	destination := filepath.Join(home, ".obsidian")
	steps := ensureShellyPkgs("aur", "obsidian-bin")
	steps = append(steps, nativeStep("Copiar configuração para ~/.obsidian", func() error {
		return installObsidian(dotfiles, home, time.Now())
	}))
	return job{
		title: "Configurar Obsidian",
		steps: steps,
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render("Configuração copiada para " + destination)
		},
	}
}
