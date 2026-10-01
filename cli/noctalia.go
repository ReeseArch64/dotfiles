package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var requiredNoctaliaPlugins = []string{
	"github-kanban",
	"llamanager",
	"mini-docker",
	"noctaproton-vpn",
	"pomodoro",
	"ssh-launcher",
	"vpn-manager",
	"warp",
	"zed-provider",
}

type noctaliaStatus struct {
	plugins []noctaliaPlugin
	links   []noctaliaLink
}

type noctaliaPlugin struct {
	name      string
	installed bool
}

type noctaliaLink struct {
	name, source, destination string
	ok                        bool
}

func noctaliaLinkDefs(dotfiles, home string) []noctaliaLink {
	sourceDir := filepath.Join(dotfiles, "noctalia")
	if absolute, err := filepath.Abs(sourceDir); err == nil {
		sourceDir = absolute
	}
	stateDir := filepath.Join(home, ".local", "state", "noctalia")
	return []noctaliaLink{
		{name: "settings.toml", source: filepath.Join(sourceDir, "settings.toml"), destination: filepath.Join(stateDir, "settings.toml")},
		{name: "state.toml", source: filepath.Join(sourceDir, "state.toml"), destination: filepath.Join(stateDir, "state.toml")},
	}
}

func loadNoctaliaStatus(dotfiles string) noctaliaStatus {
	home, _ := os.UserHomeDir()
	return loadNoctaliaStatusForHome(dotfiles, home)
}

func (s noctaliaStatus) missingPlugins() []string {
	var missing []string
	for _, plugin := range s.plugins {
		if !plugin.installed {
			missing = append(missing, plugin.name)
		}
	}
	return missing
}

func (s noctaliaStatus) card(width int) string {
	rows := make([]cardRow, 0, len(s.plugins)+len(s.links))
	for _, plugin := range s.plugins {
		color, value := colOK, plugin.name+" instalado"
		if !plugin.installed {
			color, value = colErr, plugin.name+" ausente"
		}
		rows = append(rows, cardRow{color, "Plugin", value})
	}
	for _, link := range s.links {
		color, value := colOK, link.name+" configurado"
		if !link.ok {
			color, value = colWarn, link.name+" não configurado"
		}
		rows = append(rows, cardRow{color, "Arquivo", value})
	}
	return renderCard(rows, width)
}

func (m model) noctaliaItems() []item {
	return []item{{
		title: "Configurar Noctalia",
		desc:  "Ativar settings.toml e state.toml após baixar todos os plugins",
		job:   func() job { return noctaliaJob(m.dotfiles) },
	}}
}

func installNoctalia(dotfiles, home string, now time.Time) error {
	status := loadNoctaliaStatusForHome(dotfiles, home)
	if missing := status.missingPlugins(); len(missing) > 0 {
		return errors.New("baixe os plugins ausentes no Noctalia: " + strings.Join(missing, ", "))
	}

	links := noctaliaLinkDefs(dotfiles, home)
	for _, link := range links {
		info, err := os.Stat(link.source)
		if err != nil {
			return fmt.Errorf("ler %s: %w", link.source, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s não é um arquivo regular", link.source)
		}
	}
	stateDir := filepath.Dir(links[0].destination)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("criar %s: %w", stateDir, err)
	}

	timestamp := now.Format("20060102-150405")
	type change struct {
		link   noctaliaLink
		backup string
	}
	var changes []change
	for _, link := range links {
		if target, err := os.Readlink(link.destination); err == nil && target == link.source {
			continue
		}
		change := change{link: link}
		if _, err := os.Lstat(link.destination); err == nil {
			change.backup = link.destination + ".backup-" + timestamp
			if _, err := os.Lstat(change.backup); err == nil {
				return fmt.Errorf("backup já existe: %s", change.backup)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("verificar %s: %w", change.backup, err)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("verificar %s: %w", link.destination, err)
		}
		changes = append(changes, change)
	}

	var applied []change
	rollback := func() {
		for i := len(applied) - 1; i >= 0; i-- {
			change := applied[i]
			_ = os.Remove(change.link.destination)
			if change.backup != "" {
				_ = os.Rename(change.backup, change.link.destination)
			}
		}
	}
	for _, change := range changes {
		if change.backup != "" {
			if err := os.Rename(change.link.destination, change.backup); err != nil {
				rollback()
				return fmt.Errorf("preservar %s: %w", change.link.name, err)
			}
		}
		applied = append(applied, change)
		if err := os.Symlink(change.link.source, change.link.destination); err != nil {
			rollback()
			return fmt.Errorf("configurar %s: %w", change.link.name, err)
		}
	}
	return nil
}

func loadNoctaliaStatusForHome(dotfiles, home string) noctaliaStatus {
	pluginDir := filepath.Join(home, ".local", "state", "noctalia", "plugins", "materialized", "community")
	status := noctaliaStatus{}
	for _, name := range requiredNoctaliaPlugins {
		info, err := os.Stat(filepath.Join(pluginDir, name))
		status.plugins = append(status.plugins, noctaliaPlugin{name: name, installed: err == nil && info.IsDir()})
	}
	for _, link := range noctaliaLinkDefs(dotfiles, home) {
		target, err := os.Readlink(link.destination)
		link.ok = err == nil && target == link.source
		status.links = append(status.links, link)
	}
	return status
}

func noctaliaJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	return job{
		title: "Configurar Noctalia",
		steps: []step{nativeStep("Verificar plugins e ativar configuração", func() error {
			return installNoctalia(dotfiles, home, time.Now())
		})},
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render("settings.toml e state.toml configurados")
		},
	}
}
