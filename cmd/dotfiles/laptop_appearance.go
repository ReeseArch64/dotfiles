package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func installLaptopAppearance(dotfiles, home string) error {
	files := []struct{ source, destination string }{
		{assetPath(dotfiles, "face.jpg"), filepath.Join(home, ".face")},
		{assetPath(dotfiles, "wallpapers", "laptop.jpg"), filepath.Join(home, ".config", "omarchy", "backgrounds", "dracula", "laptop.jpg")},
	}
	// Valide ambas as origens antes de substituir arquivos do usuário.
	for _, file := range files {
		info, err := os.Stat(file.source)
		if err != nil {
			return fmt.Errorf("ler %s: %w", file.source, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("imagem não é um arquivo regular: %s", file.source)
		}
	}
	for _, file := range files {
		// A cópia atômica cria os pais com MkdirAll e não cria symlinks.
		if err := copyFileAtomic(file.source, file.destination); err != nil {
			return err
		}
	}
	return nil
}

func laptopAppearanceJob(dotfiles string) job {
	return job{
		title: "Configurar aparência do laptop",
		steps: []step{nativeStep("Copiar ~/.face e wallpaper do laptop", func() error {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			return installLaptopAppearance(dotfiles, home)
		})},
		result: func() string {
			return "Foto copiada para ~/.face e wallpaper para ~/.config/omarchy/backgrounds/dracula/laptop.jpg."
		},
	}
}
