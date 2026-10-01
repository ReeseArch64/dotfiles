package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const zenBrowserPackage = "zen-browser-bin"

func validateZenBrowserConfig(home string) error {
	path := filepath.Join(home, ".config", "zen-browser")
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("acesse %s: %w; abra o Zen Browser ou restaure a configuração e tente novamente", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s não é um diretório", path)
	}
	return nil
}

func zenBrowserPrerequisiteStepsFor(home string, missing []string) []step {
	var steps []step
	if len(missing) > 0 {
		steps = append(steps, shellyInstallStep("aur", missing...))
	}
	return append(steps, nativeStep("Verificar ~/.config/zen-browser", func() error {
		return validateZenBrowserConfig(home)
	}))
}

func zenBrowserPrerequisiteSteps(home string) []step {
	return zenBrowserPrerequisiteStepsFor(home, missingPkgs(zenBrowserPackage))
}
