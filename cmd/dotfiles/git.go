package main

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
		ok := regularFileMatches(d.src, d.dst)
		if !d.copy {
			target, err := os.Readlink(d.dst)
			ok = err == nil && target == d.src
		}
		links = append(links, gitLink{
			label: d.label,
			src:   d.src,
			dst:   d.dst,
			ok:    ok,
		})
	}
	_, envErr := gitIdentityFromEnv(filepath.Join(dotfiles, ".env"))
	return gitStatus{missing: missingPkgs("git", "lazygit"), links: links, envOK: envErr == nil}
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
	for _, pkg := range []string{"git", "lazygit"} {
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
	for i, l := range g.links {
		color, val := colOK, l.label+" ✔"
		if !l.ok {
			color, val = colWarn, l.label+" (não configurado)"
		}
		kind := "Link"
		if i == 0 {
			kind = "Arquivo"
		}
		rows = append(rows, cardRow{color, kind, val})
	}
	return renderCard(rows, width)
}

func (m model) gitItems() []item {
	return []item{
		{
			title: "Configurar Git",
			desc:  "Usa o .env, instala ferramentas e configura os arquivos do Git",
			job:   func() job { return gitJob(m.dotfiles) },
		},
	}
}

func gitLinkDefs(dotfiles, home string) []struct {
	label, src, dst string
	copy            bool
} {
	return []struct {
		label, src, dst string
		copy            bool
	}{
		{"~/.gitconfig", configPath(dotfiles, "git", ".gitconfig"), filepath.Join(home, ".gitconfig"), true},
		{"~/.gitattributes", configPath(dotfiles, "git", ".gitattributes"), filepath.Join(home, ".gitattributes"), false},
		{"~/.gitignore", configPath(dotfiles, "git", ".gitignore"), filepath.Join(home, ".gitignore"), false},
		{"~/.config/git/config", configPath(dotfiles, "git", "config"), filepath.Join(home, ".config", "git", "config"), false},
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
	path := configPath(dotfiles, "git", ".gitconfig")
	signingKey, _ := readGitSigningKey(path)
	signingValue := ""
	if signingKey != "" {
		signingValue = " " + signingKey
	}
	content := fmt.Sprintf("[user]\n    signingkey =%s\n    email = %s\n    username = %s\n    name = %s\n",
		signingValue, strconv.Quote(identity.email), strconv.Quote(identity.username), strconv.Quote(identity.name))
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

func readGitSigningKey(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("ler %s: %w", path, err)
	}
	section := ""
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.ToLower(strings.TrimSpace(trimmed[1 : len(trimmed)-1]))
			continue
		}
		key, value, ok := strings.Cut(trimmed, "=")
		if section == "user" && ok && strings.EqualFold(strings.TrimSpace(key), "signingkey") {
			return strings.TrimSpace(value), nil
		}
	}
	return "", errors.New("user.signingkey ausente em " + path)
}

func writeGitSigningKey(path, signingKey string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("ler %s: %w", path, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("acessar %s: %w", path, err)
	}
	lines := strings.Split(string(content), "\n")
	section := ""
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			section = strings.ToLower(strings.TrimSpace(trimmed[1 : len(trimmed)-1]))
			continue
		}
		key, _, ok := strings.Cut(trimmed, "=")
		if section == "user" && ok && strings.EqualFold(strings.TrimSpace(key), "signingkey") {
			lines[i] = "    signingkey = " + signingKey
			found = true
			break
		}
	}
	if !found {
		return errors.New("user.signingkey ausente em " + path)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".gitconfig-signingkey-*")
	if err != nil {
		return fmt.Errorf("criar arquivo temporário: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err = tmp.WriteString(strings.Join(lines, "\n")); err == nil {
		err = tmp.Chmod(info.Mode().Perm())
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

	steps := []step{nativeStep("Gerar configs/git/.gitconfig com .env", func() error { return writeGitConfig(dotfiles) })}
	steps = append(steps, ensurePkgs("git")...)
	steps = append(steps, ensureShellyPkgs("standard", "lazygit")...)
	steps = append(steps, nativeStep("Criar ~/.config/git/", func() error {
		return os.MkdirAll(filepath.Join(home, ".config", "git"), 0755)
	}))
	for _, d := range defs {
		label, src, dst, copyTarget := d.label, d.src, d.dst, d.copy
		if copyTarget {
			steps = append(steps, nativeStep("Copiar "+label, func() error {
				return copyFileAtomic(src, dst)
			}))
			continue
		}
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
				fmt.Sprintf("%d/%d destinos do Git configurados", ok, len(g.links)))
		},
	}
}
