package main

// Tela de Git: status dos symlinks e job de configuração.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type gitStatus struct {
	missing []string
	links   []gitLink
	envOK   bool
}

type gitLink struct {
	label, src, dst string
	ok              bool
}

func loadGitStatus(dotfiles string) gitStatus {
	home, _ := os.UserHomeDir()
	defs := gitLinkDefs(dotfiles, home)
	var links []gitLink
	for _, d := range defs {
		target, err := os.Readlink(d.dst)
		links = append(links, gitLink{
			label: d.label,
			src:   d.src,
			dst:   d.dst,
			ok:    err == nil && target == d.src,
		})
	}
	_, envErr := gitIdentityFromEnv(filepath.Join(dotfiles, ".env"))
	return gitStatus{missing: missingPkgs("git", "gitflow-next-bin", "lazygit"), links: links, envOK: envErr == nil}
}

func (g gitStatus) readyForGPG() bool {
	if !g.envOK || len(g.missing) > 0 {
		return false
	}
	for _, link := range g.links {
		if !link.ok {
			return false
		}
	}
	return true
}

func (g gitStatus) card(width int) string {
	var rows []cardRow
	missing := make(map[string]bool, len(g.missing))
	for _, pkg := range g.missing {
		missing[pkg] = true
	}
	for _, pkg := range []string{"git", "gitflow-next-bin", "lazygit"} {
		color, value := colOK, pkg+" instalado"
		if missing[pkg] {
			color, value = colErr, pkg+" não instalado"
		}
		rows = append(rows, cardRow{color, "Pacote", value})
	}
	envColor, envValue := colOK, ".env pronto"
	if !g.envOK {
		envColor, envValue = colErr, ".env ausente ou incompleto"
	}
	rows = append(rows, cardRow{envColor, "Identidade", envValue})
	for _, l := range g.links {
		color, val := colOK, l.label+" ✔"
		if !l.ok {
			color, val = colWarn, l.label+" (não configurado)"
		}
		rows = append(rows, cardRow{color, "Link", val})
	}
	return renderCard(rows, width)
}

func (m model) gitItems() []item {
	return []item{
		{
			title: "Configurar Git",
			desc:  "Usa o .env, instala as ferramentas e cria 4 symlinks",
			job:   func() job { return gitJob(m.dotfiles) },
		},
	}
}

func gitLinkDefs(dotfiles, home string) []struct{ label, src, dst string } {
	return []struct{ label, src, dst string }{
		{"~/.gitconfig", filepath.Join(dotfiles, "git", ".gitconfig"), filepath.Join(home, ".gitconfig")},
		{"~/.gitattributes", filepath.Join(dotfiles, "git", ".gitattributes"), filepath.Join(home, ".gitattributes")},
		{"~/.gitignore", filepath.Join(dotfiles, "git", ".gitignore"), filepath.Join(home, ".gitignore")},
		{"~/.config/git/config", filepath.Join(dotfiles, "git", "config"), filepath.Join(home, ".config", "git", "config")},
	}
}

type gitIdentity struct {
	email, username, name string
}

func gitIdentityFromEnv(path string) (gitIdentity, error) {
	values, err := envValues(path)
	if err != nil {
		return gitIdentity{}, err
	}
	identity := gitIdentity{
		email:    values["GIT_USER_EMAIL"],
		username: values["GIT_USERNAME"],
		name:     values["GIT_USER_NAME"],
	}
	var missing []string
	if identity.email == "" {
		missing = append(missing, "GIT_USER_EMAIL")
	}
	if identity.username == "" {
		missing = append(missing, "GIT_USERNAME")
	}
	if identity.name == "" {
		missing = append(missing, "GIT_USER_NAME")
	}
	if len(missing) > 0 {
		return gitIdentity{}, errors.New("variáveis ausentes no .env: " + strings.Join(missing, ", "))
	}
	return identity, nil
}

func writeGitConfig(dotfiles string) error {
	identity, err := gitIdentityFromEnv(filepath.Join(dotfiles, ".env"))
	if err != nil {
		return err
	}
	content := fmt.Sprintf("[user]\n    signingkey =\n    email = %s\n    username = %s\n    name = %s\n",
		strconv.Quote(identity.email), strconv.Quote(identity.username), strconv.Quote(identity.name))
	path := filepath.Join(dotfiles, "git", ".gitconfig")
	tmp, err := os.CreateTemp(filepath.Dir(path), ".gitconfig-*")
	if err != nil {
		return fmt.Errorf("criar arquivo temporário: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err = tmp.WriteString(content); err == nil {
		err = tmp.Chmod(0644)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("gravar %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("substituir %s: %w", path, err)
	}
	return nil
}

func gitJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	defs := gitLinkDefs(dotfiles, home)

	steps := []step{nativeStep("Gerar git/.gitconfig com .env", func() error { return writeGitConfig(dotfiles) })}
	steps = append(steps, ensurePkgs("git")...)
	steps = append(steps, ensureShellyPkgs("aur", "gitflow-next-bin")...)
	steps = append(steps, ensureShellyPkgs("standard", "lazygit")...)
	steps = append(steps, nativeStep("Criar ~/.config/git/", func() error {
		return os.MkdirAll(filepath.Join(home, ".config", "git"), 0755)
	}))
	for _, d := range defs {
		label, src, dst := d.label, d.src, d.dst
		steps = append(steps, nativeStep("Symlink "+label, func() error {
			os.Remove(dst)
			return os.Symlink(src, dst)
		}))
	}

	return job{
		title: "Configurar Git",
		steps: steps,
		result: func() string {
			g := loadGitStatus(dotfiles)
			ok := 0
			for _, l := range g.links {
				if l.ok {
					ok++
				}
			}
			return lipgloss.NewStyle().Foreground(colOK).Render(
				fmt.Sprintf("%d/%d symlinks configurados", ok, len(g.links)))
		},
	}
}
