package main

// Tela de Git: status dos symlinks e job de configuração.

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
)

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
		target, err := os.Readlink(d.dst)
		links = append(links, gitLink{
			label: d.label,
			src:   d.src,
			dst:   d.dst,
			ok:    err == nil && target == d.src,
		})
	}
	return gitStatus{missing: missingPkgs("git"), links: links}
}

func (g gitStatus) card(width int) string {
	var rows []cardRow
	if len(g.missing) > 0 {
		rows = append(rows, cardRow{colErr, "Pacote", "git não instalado"})
	} else {
		rows = append(rows, cardRow{colOK, "Pacote", "git instalado"})
	}
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
			desc:  "Instala git (se necessário) e cria 4 symlinks no home",
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

func gitJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	defs := gitLinkDefs(dotfiles, home)

	steps := ensurePkgs("git")
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
