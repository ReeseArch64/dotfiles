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

type gpgStatus struct {
	gnupgInstalled bool
	gitReady       bool
	signingKey     string
	configErr      error
}

func loadGPGStatus(setupDir string) gpgStatus {
	signingKey, err := readGitSigningKey(configPath(setupDir, "git", ".gitconfig"))
	return gpgStatus{
		gnupgInstalled: len(missingPkgs("gnupg")) == 0,
		gitReady:       loadGitStatus(setupDir).readyForGPG(),
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
	signingColor, signingValue := colOK, g.signingKey
	if signingValue == "" || g.configErr != nil {
		signingColor, signingValue = colWarn, "não configurada"
	}
	return renderCard([]cardRow{
		{packageColor, "Pacote", packageValue},
		{gitColor, "Requisito", gitValue},
		{colOK, "Chave", "gerada localmente por dispositivo"},
		{signingColor, "Assinatura", signingValue},
	}, width)
}

func (m model) gpgItems() []item {
	return []item{{
		title: "Gerar chave por dispositivo",
		desc:  "Cria chave com senha, exibe a pública e configura user.signingkey",
		job:   func() job { return gpgGenerateJob(m.setupDir) },
	}}
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

func parseGPGCreatedFingerprint(output string) (string, error) {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 4 && fields[0] == "[GNUPG:]" && fields[1] == "KEY_CREATED" &&
			(fields[2] == "P" || fields[2] == "B") && validGPGFingerprint(fields[3]) {
			return fields[3], nil
		}
	}
	return "", errors.New("fingerprint da chave gerada não encontrado")
}

func gpgDeviceIdentity(setupDir, hostname string) (string, error) {
	var values []string
	for _, field := range []string{"user.name", "user.email"} {
		output, err := exec.Command("git", "config", "--file", configPath(setupDir, "git", ".gitconfig"), "--get", field).Output()
		if err != nil {
			return "", fmt.Errorf("ler %s do Git: %w", field, err)
		}
		value := strings.TrimSpace(string(output))
		if value == "" || strings.ContainsAny(value, "\r\n\x00<>()") {
			return "", fmt.Errorf("%s inválido para a identidade GPG", field)
		}
		values = append(values, value)
	}
	if hostname == "" || strings.ContainsAny(hostname, "\r\n\x00<>()") {
		return "", errors.New("nome do dispositivo inválido para a identidade GPG")
	}
	return fmt.Sprintf("%s (%s) <%s>", values[0], hostname, values[1]), nil
}

func applyGitSigningKey(setupDir, home, fingerprint string) error {
	source := configPath(setupDir, "git", ".gitconfig")
	if err := writeGitSigningKey(source, fingerprint); err != nil {
		return err
	}
	return copyFileAtomic(source, filepath.Join(home, ".gitconfig"))
}

// A senha existe apenas na memória do shell e no descritor entregue ao GnuPG.
// Nunca é passada por argumento, variável de ambiente ou arquivo temporário.
const gpgGenerateScript = `set -e
status=$1
identity=$2
unset passphrase confirmation
trap 'code=$?; unset passphrase confirmation; if [ "$code" -ne 0 ]; then rm -f -- "$status"; fi' EXIT
printf 'Uma nova chave de assinatura será criada para %s.\n' "$identity"
while true; do
    IFS= read -r -s -p 'Senha da nova chave GPG: ' passphrase
    printf '\n'
    IFS= read -r -s -p 'Confirme a senha: ' confirmation
    printf '\n'
    if [ -z "$passphrase" ]; then
        printf 'A senha não pode ficar vazia.\n' >&2
    elif [ "$passphrase" != "$confirmation" ]; then
        printf 'As senhas não coincidem. Tente novamente.\n' >&2
    else
        break
    fi
done
printf '%s\n' "$passphrase" | gpg --batch --pinentry-mode loopback --passphrase-fd 0 --status-file "$status" \
    --quick-generate-key "$identity" ed25519 sign 2y
`

const gpgShowPublicScript = `set -e
printf 'Copie TODO o bloco abaixo e cadastre como chave GPG no GitHub, GitLab e Gitea.\n\n'
gpg --batch --armor --export "$1"
printf '\nFingerprint: %s\n' "$1"
read -r -p 'Após copiar a chave pública, pressione Enter para configurar a assinatura…'`

func gpgGenerateSteps(setupDir, home string, packageSteps []step) []step {
	var identity, statusPath, fingerprint string
	steps := []step{nativeStep("Validar Git e identidade do dispositivo", func() error {
		if !loadGitStatus(setupDir).readyForGPG() {
			return errors.New("configure o Git antes de gerar a chave GPG")
		}
		hostname, err := os.Hostname()
		if err != nil {
			return fmt.Errorf("identificar dispositivo: %w", err)
		}
		identity, err = gpgDeviceIdentity(setupDir, hostname)
		return err
	})}
	steps = append(steps, packageSteps...)
	steps = append(steps,
		nativeStep("Preparar geração GPG", func() error {
			file, err := os.CreateTemp("", "dotfiles-gpg-status-*")
			if err != nil {
				return err
			}
			statusPath = file.Name()
			return file.Close()
		}),
		step{label: "Gerar chave GPG com senha", cmd: func() *exec.Cmd {
			return terminalStep("Gerar chave GPG com senha", "bash", "-c", gpgGenerateScript, "dotfiles-gpg", statusPath, identity).cmd()
		}},
		nativeStep("Identificar chave gerada", func() error {
			defer os.Remove(statusPath)
			output, err := os.ReadFile(statusPath)
			if err != nil {
				return err
			}
			fingerprint, err = parseGPGCreatedFingerprint(string(output))
			return err
		}),
		step{label: "Exibir chave pública para GitHub, GitLab e Gitea", cmd: func() *exec.Cmd {
			return terminalStep("Chave pública GPG", "bash", "-c", gpgShowPublicScript, "dotfiles-gpg-public", fingerprint).cmd()
		}},
		nativeStep("Configurar user.signingkey", func() error {
			if !validGPGFingerprint(fingerprint) {
				return errors.New("fingerprint da chave gerada inválido")
			}
			return applyGitSigningKey(setupDir, home, fingerprint)
		}),
	)
	return steps
}

func gpgGenerateJob(setupDir string) job {
	home, _ := os.UserHomeDir()
	return job{
		title: "Gerar chave GPG por dispositivo",
		steps: gpgGenerateSteps(setupDir, home, ensurePkgs("gnupg")),
		result: func() string {
			signingKey, _ := readGitSigningKey(configPath(setupDir, "git", ".gitconfig"))
			return lipgloss.NewStyle().Foreground(colOK).Render("Chave local gerada · signingkey " + signingKey)
		},
	}
}
