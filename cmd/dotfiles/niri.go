package main

import (
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func installNiri(setupDir, home string, now time.Time) error {
	if err := os.MkdirAll(filepath.Join(home, "Pictures", "Screenshots"), 0755); err != nil {
		return err
	}
	return copyConfigDirectoryWithBackup(setupDir, filepath.Join(home, ".config", "niri"), now, "niri")
}

func installNiriCursor(setupDir, home string, now time.Time) error {
	return copyConfigDirectoryWithBackup(setupDir, filepath.Join(home, ".icons", "FrierenBLZ"), now, "cursor", "FrierenBLZ")
}

func niriJobFor(setupDir, home string, now func() time.Time) job {
	return job{
		title: "Configurar Niri",
		steps: []step{
			nativeStep("Copiar configuração para ~/.config/niri", func() error {
				return installNiri(setupDir, home, now())
			}),
			nativeStep("Copiar cursor FrierenBLZ para ~/.icons", func() error {
				return installNiriCursor(setupDir, home, now())
			}),
		},
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render("Configuração do Niri e cursor FrierenBLZ instalados.")
		},
	}
}

func niriJob(setupDir string) job {
	home, _ := os.UserHomeDir()
	return niriJobFor(setupDir, home, time.Now)
}
