package main

import (
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func installWallpapers(dotfiles, home string, now time.Time) error {
	return copyDirectoryWithBackup(
		assetPath(dotfiles, "wallpapers"),
		filepath.Join(home, ".wallpapers"),
		now,
	)
}

func wallpapersJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	destination := filepath.Join(home, ".wallpapers")
	return job{
		title: "Configurar wallpapers",
		steps: []step{nativeStep("Copiar imagens para ~/.wallpapers", func() error {
			return installWallpapers(dotfiles, home, time.Now())
		})},
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render("Wallpapers copiados para " + destination)
		},
	}
}
