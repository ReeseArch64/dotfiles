package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	signingKey     string
	configErr      error
}

func loadGPGStatus(dotfiles string) gpgStatus {
	paths, err := gpgImportPathsFromEnv(filepath.Join(dotfiles, ".env"))
	signingKey, _ := readGitSigningKey(configPath(dotfiles, "git", ".gitconfig"))
	return gpgStatus{
		gnupgInstalled: len(missingPkgs("gnupg")) == 0,
		gitReady:       loadGitStatus(dotfiles).readyForGPG(),
		paths:          paths,
		signingKey:     signingKey,
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
	signingColor, signingValue := colOK, g.signingKey
	if signingValue == "" {
		signingColor, signingValue = colWarn, "não configurada"
	}
	return renderCard([]cardRow{
		{packageColor, "Pacote", packageValue},
		{gitColor, "Requisito", gitValue},
		{configColor, "Pública", publicValue},
		{configColor, "Privada", privateValue},
		{signingColor, "Assinatura", signingValue},
	}, width)
}

func (m model) gpgItems() []item {
	return []item{
		{
			title: "Importar chaves",
			desc:  "Importa as chaves .asc e configura user.signingkey",
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
	keys := []struct {
		label, path, header string
	}{
		{"pública", paths.public, "-----BEGIN PGP PUBLIC KEY BLOCK-----"},
		{"privada", paths.private, "-----BEGIN PGP PRIVATE KEY BLOCK-----"},
	}
	for _, key := range keys {
		if !strings.EqualFold(filepath.Ext(key.path), ".asc") {
			return gpgImportPaths{}, fmt.Errorf("a chave %s deve usar a extensão .asc: %s", key.label, key.path)
		}
		info, err := os.Stat(key.path)
		if err != nil {
			return gpgImportPaths{}, fmt.Errorf("acessar chave %s %s: %w", key.label, key.path, err)
		}
		if !info.Mode().IsRegular() {
			return gpgImportPaths{}, fmt.Errorf("a chave %s não é um arquivo regular: %s", key.label, key.path)
		}
		content, err := os.ReadFile(key.path)
		if err != nil {
			return gpgImportPaths{}, fmt.Errorf("ler chave %s %s: %w", key.label, key.path, err)
		}
		if !strings.Contains(string(content), key.header) {
			return gpgImportPaths{}, fmt.Errorf("a chave %s não contém um bloco OpenPGP válido: %s", key.label, key.path)
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

func parseGPGSecretFingerprint(output string) (string, error) {
	secretKey := false
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Split(line, ":")
		if len(fields) < 10 {
			continue
		}
		switch fields[0] {
		case "sec":
			secretKey = true
		case "fpr":
			if secretKey && validGPGFingerprint(fields[9]) {
				return fields[9], nil
			}
		case "pub", "ssb", "sub":
			secretKey = false
		}
	}
	return "", errors.New("fingerprint da chave privada não encontrado")
}

func validGPGFingerprint(value string) bool {
	if len(value) != 32 && len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
			return false
		}
	}
	return true
}

func signingKeyFromPrivateFile(path string) (string, error) {
	output, err := exec.Command("gpg", "--batch", "--with-colons", "--import-options", "show-only", "--import", path).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("identificar chave privada: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return parseGPGSecretFingerprint(string(output))
}

func applyGitSigningKey(dotfiles, home, fingerprint string) error {
	source := configPath(dotfiles, "git", ".gitconfig")
	if err := writeGitSigningKey(source, fingerprint); err != nil {
		return err
	}
	return copyFileAtomic(source, filepath.Join(home, ".gitconfig"))
}

func configureGitSigningKey(dotfiles, privateKeyPath string) error {
	fingerprint, err := signingKeyFromPrivateFile(privateKeyPath)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("identificar diretório pessoal: %w", err)
	}
	return applyGitSigningKey(dotfiles, home, fingerprint)
}

func gpgImportSteps(dotfiles string, paths gpgImportPaths, pathErr error, packageSteps []step) []step {
	steps := []step{nativeStep("Validar Git e arquivos .asc", func() error {
		return validateGPGImport(dotfiles, pathErr)
	})}
	steps = append(steps, packageSteps...)
	return append(steps,
		terminalStep("Importar chave pública", "gpg", "--import", paths.public),
		terminalStep("Importar chave privada", "gpg", "--import", paths.private),
		nativeStep("Configurar user.signingkey", func() error {
			return configureGitSigningKey(dotfiles, paths.private)
		}),
	)
}

func gpgImportJob(dotfiles string) job {
	paths, pathErr := gpgImportPathsFromEnv(filepath.Join(dotfiles, ".env"))
	steps := gpgImportSteps(dotfiles, paths, pathErr, ensurePkgs("gnupg"))
	return job{
		title: "Importar chaves GPG",
		steps: steps,
		result: func() string {
			signingKey, _ := readGitSigningKey(configPath(dotfiles, "git", ".gitconfig"))
			return lipgloss.NewStyle().Foreground(colOK).Render("Chaves importadas · signingkey " + signingKey)
		},
	}
}
