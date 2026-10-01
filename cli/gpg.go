package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type gpgImportPaths struct {
	public  string
	private string
}

type gpgStatus struct {
	gnupgInstalled bool
	gitReady       bool
	paths          gpgImportPaths
	configErr      error
}

func loadGPGStatus(dotfiles string) gpgStatus {
	paths, err := gpgImportPathsFromEnv(filepath.Join(dotfiles, ".env"))
	return gpgStatus{
		gnupgInstalled: len(missingPkgs("gnupg")) == 0,
		gitReady:       loadGitStatus(dotfiles).readyForGPG(),
		paths:          paths,
		configErr:      err,
	}
}

func (g gpgStatus) card(width int) string {
	packageColor, packageValue := colOK, "gnupg instalado"
	if !g.gnupgInstalled {
		packageColor, packageValue = colErr, "gnupg não instalado"
	}
	gitColor, gitValue := colOK, "Git configurado"
	if !g.gitReady {
		gitColor, gitValue = colErr, "configure o Git primeiro"
	}
	configColor, publicValue, privateValue := colOK, filepath.Base(g.paths.public), filepath.Base(g.paths.private)
	if g.configErr != nil {
		configColor, publicValue, privateValue = colErr, "chave pública inválida", "chave privada inválida"
	}
	return renderCard([]cardRow{
		{packageColor, "Pacote", packageValue},
		{gitColor, "Requisito", gitValue},
		{configColor, "Pública", publicValue},
		{configColor, "Privada", privateValue},
	}, width)
}

func (m model) gpgItems() []item {
	return []item{
		{
			title: "Importar chaves",
			desc:  "Importa as chaves pública e privada .asc definidas no .env",
			job:   func() job { return gpgImportJob(m.dotfiles) },
		},
	}
}

func gpgImportPathsFromEnv(path string) (gpgImportPaths, error) {
	values, err := envValues(path)
	if err != nil {
		return gpgImportPaths{}, err
	}
	paths := gpgImportPaths{
		public:  expandHome(values["GPG_PUBLIC_IMPORT"]),
		private: expandHome(values["GPG_PRIVATE_IMPORT"]),
	}
	var missing []string
	if paths.public == "" {
		missing = append(missing, "GPG_PUBLIC_IMPORT")
	}
	if paths.private == "" {
		missing = append(missing, "GPG_PRIVATE_IMPORT")
	}
	if len(missing) > 0 {
		return gpgImportPaths{}, errors.New("variáveis ausentes no .env: " + strings.Join(missing, ", "))
	}
	if paths.public == paths.private {
		return gpgImportPaths{}, errors.New("as chaves pública e privada devem ser arquivos diferentes")
	}
	for label, file := range map[string]string{"pública": paths.public, "privada": paths.private} {
		if !strings.EqualFold(filepath.Ext(file), ".asc") {
			return gpgImportPaths{}, fmt.Errorf("a chave %s deve usar a extensão .asc: %s", label, file)
		}
		info, err := os.Stat(file)
		if err != nil {
			return gpgImportPaths{}, fmt.Errorf("acessar chave %s %s: %w", label, file, err)
		}
		if !info.Mode().IsRegular() {
			return gpgImportPaths{}, fmt.Errorf("a chave %s não é um arquivo regular: %s", label, file)
		}
	}
	return paths, nil
}

func validateGPGImport(dotfiles string, pathErr error) error {
	if !loadGitStatus(dotfiles).readyForGPG() {
		return errors.New("configure o Git antes de importar as chaves GPG")
	}
	return pathErr
}

func gpgImportSteps(dotfiles string, paths gpgImportPaths, pathErr error, packageSteps []step) []step {
	steps := []step{nativeStep("Validar Git e arquivos .asc", func() error {
		return validateGPGImport(dotfiles, pathErr)
	})}
	steps = append(steps, packageSteps...)
	return append(steps,
		terminalStep("Importar chave pública", "gpg", "--import", paths.public),
		terminalStep("Importar chave privada", "gpg", "--import", paths.private),
	)
}

func gpgImportJob(dotfiles string) job {
	paths, pathErr := gpgImportPathsFromEnv(filepath.Join(dotfiles, ".env"))
	steps := gpgImportSteps(dotfiles, paths, pathErr, ensurePkgs("gnupg"))
	return job{
		title: "Importar chaves GPG",
		steps: steps,
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render("Chaves pública e privada importadas")
		},
	}
}
