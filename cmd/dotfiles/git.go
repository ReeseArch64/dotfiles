package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var gitPacmanPackages = []string{"git", "github-cli"}
var gitShellyPackages = []string{"lazygit", "glab"}

type gitStatus struct {
	missing []string
	links   []gitLink
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
	return gitStatus{missing: missingPkgs("git", "github-cli", "lazygit", "glab"), links: links}
}

func (g gitStatus) readyForGPG() bool {
	if len(g.missing) > 0 {
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
	for _, pkg := range []string{"git", "github-cli", "lazygit", "glab"} {
		color, value := colOK, pkg+" instalado"
		if missing[pkg] {
			color, value = colErr, pkg+" não instalado"
		}
		rows = append(rows, cardRow{color, "Pacote", value})
	}
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
			desc:  "Instala ferramentas e aplica os arquivos globais do Git",
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

func validateGitSSH(dotfiles, home string) error {
	if missing := missingSSHIdentityFiles(home); len(missing) > 0 {
		return errors.New("configure o SSH antes de autenticar GitHub e GitLab: chaves ausentes: " + strings.Join(missing, ", "))
	}
	source := configPath(dotfiles, "ssh", "config")
	destination := filepath.Join(home, ".ssh", "config")
	if !regularFileMatches(source, destination) {
		return errors.New("configure o SSH antes de autenticar GitHub e GitLab: ~/.ssh/config não está configurado")
	}
	return nil
}

func githubLoginStep() step {
	return terminalStep("Autenticar no GitHub com SSH", "gh", "auth", "login", "--git-protocol", "ssh")
}

func glabLoginStep() step {
	return terminalStep("Autenticar no GitLab com SSH", "glab", "auth", "login", "--git-protocol", "ssh")
}

func gitJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	defs := gitLinkDefs(dotfiles, home)

	steps := ensurePkgs(gitPacmanPackages...)
	steps = append(steps, ensureShellyPkgs("standard", gitShellyPackages...)...)
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
	steps = append(steps,
		nativeStep("Validar SSH para GitHub e GitLab", func() error {
			return validateGitSSH(dotfiles, home)
		}),
		skipWhen(githubLoginStep(), func() bool { return commandSucceeds(nil, "gh", "auth", "status") }),
		skipWhen(glabLoginStep(), func() bool { return commandSucceeds(nil, "glab", "auth", "status") }),
	)

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
