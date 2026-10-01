package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func checkPlatform(osReleasePath string, lookPath func(string) (string, error)) error {
	id, err := osReleaseID(osReleasePath)
	var missing []string
	if err != nil || id != "cachyos" {
		missing = append(missing, "CachyOS")
	}
	if _, err := lookPath("niri"); err != nil {
		missing = append(missing, "Niri")
	}
	if _, err := lookPath("noctalia"); err != nil {
		missing = append(missing, "Noctalia Shell")
	}
	if len(missing) > 0 {
		return fmt.Errorf("ambiente incompatível: requer CachyOS -> Niri -> Noctalia Shell; ausente: %s", strings.Join(missing, ", "))
	}
	return nil
}

func checkPrerequisites(dotfiles, home string, lookPath func(string) (string, error)) error {
	var issues []string
	if _, err := lookPath("zen-browser"); err != nil {
		issues = append(issues, "Zen Browser (zen-browser) não instalado")
	}
	zenConfig := filepath.Join(home, ".config", "zen-browser")
	if info, err := os.Stat(zenConfig); err != nil || !info.IsDir() {
		issues = append(issues, zenConfig+" ausente ou inválido")
	}
	keys := []struct {
		name, header string
	}{
		{"minha_chave_privada.asc", "-----BEGIN PGP PRIVATE KEY BLOCK-----"},
		{"minha_chave_publica.asc", "-----BEGIN PGP PUBLIC KEY BLOCK-----"},
	}
	for _, key := range keys {
		path := filepath.Join(dotfiles, key.name)
		content, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(content), key.header) {
			issues = append(issues, path+" ausente ou inválido")
		}
	}
	if len(issues) > 0 {
		return errors.New("pré-requisitos ausentes: " + strings.Join(issues, "; "))
	}
	return nil
}

func osReleaseID(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if ok && key == "ID" {
			return strings.Trim(value, `"'`), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("ID ausente em os-release")
}
