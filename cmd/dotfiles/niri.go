package main

import (
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func installNiri(dotfiles, home string, now time.Time) error {
	return copyDirectoryWithBackup(
		configPath(dotfiles, "niri"),
		filepath.Join(home, ".config", "niri"),
		now,
	)
}

func installNiriCursor(dotfiles, home string, now time.Time) error {
	return copyDirectoryWithBackup(
		configPath(dotfiles, "cursor", "FrierenBLZ"),
		filepath.Join(home, ".icons", "FrierenBLZ"),
		now,
	)
}

func niriJobFor(dotfiles, home string, now func() time.Time) job {
	return job{
		title: "Configurar Niri",
		steps: []step{
			nativeStep("Copiar configuração para ~/.config/niri", func() error {
				return installNiri(dotfiles, home, now())
			}),
			nativeStep("Copiar cursor FrierenBLZ para ~/.icons", func() error {
				return installNiriCursor(dotfiles, home, now())
			}),
		},
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render("Configuração do Niri e cursor FrierenBLZ instalados.")
		},
	}
}

func niriJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	return niriJobFor(dotfiles, home, time.Now)
}
