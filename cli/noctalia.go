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
	files   []noctaliaFile
}

type noctaliaPlugin struct {
	name      string
	installed bool
}

type noctaliaFile struct {
	name, source, destination string
	ok                        bool
}

func noctaliaFileDefs(dotfiles, home string) []noctaliaFile {
	sourceDir := filepath.Join(dotfiles, "noctalia")
	if absolute, err := filepath.Abs(sourceDir); err == nil {
		sourceDir = absolute
	}
	stateDir := filepath.Join(home, ".local", "state", "noctalia")
	return []noctaliaFile{
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
	rows := make([]cardRow, 0, len(s.plugins)+len(s.files))
	for _, plugin := range s.plugins {
		color, value := colOK, plugin.name+" instalado"
		if !plugin.installed {
			color, value = colErr, plugin.name+" ausente"
		}
		rows = append(rows, cardRow{color, "Plugin", value})
	}
	for _, file := range s.files {
		color, value := colOK, file.name+" configurado"
		if !file.ok {
			color, value = colWarn, file.name+" não configurado"
		}
		rows = append(rows, cardRow{color, "Arquivo", value})
	}
	return renderCard(rows, width)
}

func (m model) noctaliaItems() []item {
	return []item{{
		title: "Configurar Noctalia",
		desc:  "Copiar settings.toml e state.toml após baixar todos os plugins",
		job:   func() job { return noctaliaJob(m.dotfiles) },
	}}
}

func installNoctalia(dotfiles, home string, now time.Time) error {
	status := loadNoctaliaStatusForHome(dotfiles, home)
	if missing := status.missingPlugins(); len(missing) > 0 {
		return errors.New("baixe os plugins ausentes no Noctalia: " + strings.Join(missing, ", "))
	}

	files := noctaliaFileDefs(dotfiles, home)
	for _, file := range files {
		info, err := os.Stat(file.source)
		if err != nil {
			return fmt.Errorf("ler %s: %w", file.source, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s não é um arquivo regular", file.source)
		}
	}
	stateDir := filepath.Dir(files[0].destination)
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("criar %s: %w", stateDir, err)
	}

	timestamp := now.Format("20060102-150405")
	type change struct {
		file   noctaliaFile
		backup string
	}
	var changes []change
	for _, file := range files {
		if regularFileMatches(file.source, file.destination) {
			continue
		}
		change := change{file: file}
		if _, err := os.Lstat(file.destination); err == nil {
			change.backup = file.destination + ".backup-" + timestamp
			if _, err := os.Lstat(change.backup); err == nil {
				return fmt.Errorf("backup já existe: %s", change.backup)
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("verificar %s: %w", change.backup, err)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("verificar %s: %w", file.destination, err)
		}
		changes = append(changes, change)
	}

	var applied []change
	rollback := func() {
		for i := len(applied) - 1; i >= 0; i-- {
			change := applied[i]
			_ = os.Remove(change.file.destination)
			if change.backup != "" {
				_ = os.Rename(change.backup, change.file.destination)
			}
		}
	}
	for _, change := range changes {
		if change.backup != "" {
			if err := os.Rename(change.file.destination, change.backup); err != nil {
				rollback()
				return fmt.Errorf("preservar %s: %w", change.file.name, err)
			}
		}
		applied = append(applied, change)
		if err := copyFileAtomic(change.file.source, change.file.destination); err != nil {
			rollback()
			return fmt.Errorf("configurar %s: %w", change.file.name, err)
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
	for _, file := range noctaliaFileDefs(dotfiles, home) {
		file.ok = regularFileMatches(file.source, file.destination)
		status.files = append(status.files, file)
	}
	return status
}

func noctaliaJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	steps := zenBrowserPrerequisiteSteps(home)
	steps = append(steps, nativeStep("Verificar plugins e copiar configuração", func() error {
		return installNoctalia(dotfiles, home, time.Now())
	}))
	return job{
		title: "Configurar Noctalia",
		steps: steps,
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render("settings.toml e state.toml configurados")
		},
	}
}
