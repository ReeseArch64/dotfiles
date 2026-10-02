package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
)

func installFace(dotfiles, home string) error {
	source, err := filepath.Abs(assetPath(dotfiles, "face.jpg"))
	if err != nil {
		return fmt.Errorf("resolver .face: %w", err)
	}
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("ler %s: %w", source, err)
	}
	destination := filepath.Join(home, ".face")
	if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remover %s: %w", destination, err)
	}
	if err := os.Symlink(source, destination); err != nil {
		return fmt.Errorf("criar %s: %w", destination, err)
	}
	return nil
}

func faceJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	return job{
		title: "Configurar foto de perfil",
		steps: []step{nativeStep("Criar symlink ~/.face", func() error {
			return installFace(dotfiles, home)
		})},
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render("~/.face aponta para " + assetPath(dotfiles, "face.jpg"))
		},
	}
}
