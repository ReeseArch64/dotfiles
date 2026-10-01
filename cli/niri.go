package main

import (
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func installNiri(dotfiles, home string, now time.Time) error {
	return copyDirectoryWithBackup(
		filepath.Join(dotfiles, "niri"),
		filepath.Join(home, ".config", "niri"),
		now,
	)
}

func niriJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	destination := filepath.Join(home, ".config", "niri")
	steps := zenBrowserPrerequisiteSteps(home)
	steps = append(steps, nativeStep("Copiar configuração para ~/.config/niri", func() error {
		return installNiri(dotfiles, home, time.Now())
	}))
	return job{
		title: "Configurar Niri",
		steps: steps,
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render("Configuração copiada para " + destination)
		},
	}
}
