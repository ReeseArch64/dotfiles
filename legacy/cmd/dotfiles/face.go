package main

import (
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
)

func installFace(dotfiles, home string) error {
	return copyFileAtomic(assetPath(dotfiles, "face.jpg"), filepath.Join(home, ".face"))
}

func faceJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	return job{
		title: "Configurar foto de perfil",
		steps: []step{nativeStep("Copiar foto para ~/.face", func() error {
			return installFace(dotfiles, home)
		})},
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render("Foto copiada para ~/.face")
		},
	}
}
