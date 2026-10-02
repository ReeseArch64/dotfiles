package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var requiredPiPackages = []string{
	"https://github.com/nothingrotf/pi-extensions",
	"git/github.com/nothingrotf/pi-extensions/packages/ask",
	"git/github.com/nothingrotf/pi-extensions/packages/compact",
	"git/github.com/nothingrotf/pi-extensions/packages/fast-mode",
	"git/github.com/nothingrotf/pi-extensions/packages/filetools",
	"git/github.com/nothingrotf/pi-extensions/packages/goal",
	"git/github.com/nothingrotf/pi-extensions/packages/hud",
	"git/github.com/nothingrotf/pi-extensions/packages/inline-skill",
	"git/github.com/nothingrotf/pi-extensions/packages/loop",
	"git/github.com/nothingrotf/pi-extensions/packages/session-history",
	"git/github.com/nothingrotf/pi-extensions/packages/subagent",
	"git/github.com/nothingrotf/pi-extensions/packages/tgrep",
	"git/github.com/nothingrotf/pi-extensions/packages/todo",
	"git/github.com/nothingrotf/pi-extensions/packages/pstack",
	"npm:@gotgenes/pi-anthropic-auth",
	"npm:pi-antigravity",
}

type piAgentStatus struct {
	piAvailable bool
	settingsOK  bool
	themeOK     bool
	installed   int
	total       int
	configErr   error
}

type piSettingsFile struct {
	Theme    string   `json:"theme"`
	Packages []string `json:"packages"`
}

type piThemeFile struct {
	Name string `json:"name"`
}

type piConfigFile struct {
	name, source, destination string
}

func piAgentDir(home string) string {
	return filepath.Join(home, ".pi", "agent")
}

func piConfigFiles(dotfiles, home string) []piConfigFile {
	agentDir := piAgentDir(home)
	return []piConfigFile{
		{name: "settings.json", source: configPath(dotfiles, "pi", "agent", "settings.json"), destination: filepath.Join(agentDir, "settings.json")},
		{name: "noctalia.json", source: configPath(dotfiles, "pi", "agent", "themes", "noctalia.json"), destination: filepath.Join(agentDir, "themes", "noctalia.json")},
	}
}

func validatePiConfig(dotfiles string) error {
	settingsPath := configPath(dotfiles, "pi", "agent", "settings.json")
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		return fmt.Errorf("ler %s: %w", settingsPath, err)
	}
	var settings piSettingsFile
	if err := json.Unmarshal(content, &settings); err != nil {
		return fmt.Errorf("validar %s: %w", settingsPath, err)
	}
	if settings.Theme != "noctalia" {
		return fmt.Errorf("tema esperado em settings.json: noctalia")
	}
	if !slices.Equal(settings.Packages, requiredPiPackages) {
		return errors.New("a lista de pacotes do Pi não corresponde à configuração esperada")
	}
	themePath := configPath(dotfiles, "pi", "agent", "themes", "noctalia.json")
	content, err = os.ReadFile(themePath)
	if err != nil {
		return fmt.Errorf("ler %s: %w", themePath, err)
	}
	var theme piThemeFile
	if err := json.Unmarshal(content, &theme); err != nil {
		return fmt.Errorf("validar %s: %w", themePath, err)
	}
	if theme.Name != "noctalia" {
		return errors.New("o tema deve usar o nome noctalia")
	}
	return nil
}

func piPackagePath(agentDir, source string) string {
	switch {
	case source == "https://github.com/nothingrotf/pi-extensions":
		return filepath.Join(agentDir, "git", "github.com", "nothingrotf", "pi-extensions")
	case strings.HasPrefix(source, "git/"):
		return filepath.Join(agentDir, filepath.FromSlash(source))
	case strings.HasPrefix(source, "npm:"):
		return filepath.Join(agentDir, "npm", "node_modules", filepath.FromSlash(strings.TrimPrefix(source, "npm:")))
	default:
		return ""
	}
}

func executableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0
}

func loadPiAgentStatusForHome(dotfiles, home string, lookPath func(string) (string, error)) piAgentStatus {
	files := piConfigFiles(dotfiles, home)
	status := piAgentStatus{total: len(requiredPiPackages), configErr: validatePiConfig(dotfiles)}
	_, err := lookPath("pi")
	status.piAvailable = err == nil || executableFile(filepath.Join(piAgentDir(home), "bin", "pi"))
	status.settingsOK = regularFileMatches(files[0].source, files[0].destination)
	status.themeOK = regularFileMatches(files[1].source, files[1].destination)
	agentDir := piAgentDir(home)
	for _, source := range requiredPiPackages {
		path := piPackagePath(agentDir, source)
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			status.installed++
		}
	}
	return status
}

func loadPiAgentStatus(dotfiles string) piAgentStatus {
	home, _ := os.UserHomeDir()
	return loadPiAgentStatusForHome(dotfiles, home, exec.LookPath)
}

func (s piAgentStatus) card(width int) string {
	piColor, piValue := colOK, "pi instalado"
	if !s.piAvailable {
		piColor, piValue = colWarn, "pi será instalado"
	}
	settingsColor, settingsValue := colOK, "settings.json configurado"
	if !s.settingsOK {
		settingsColor, settingsValue = colWarn, "settings.json não configurado"
	}
	themeColor, themeValue := colOK, "tema noctalia configurado"
	if !s.themeOK {
		themeColor, themeValue = colWarn, "tema noctalia não configurado"
	}
	packageColor := colOK
	if s.installed != s.total {
		packageColor = colWarn
	}
	rows := []cardRow{
		{piColor, "CLI", piValue},
		{settingsColor, "Settings", settingsValue},
		{themeColor, "Tema", themeValue},
		{packageColor, "Pacotes", fmt.Sprintf("%d/%d instalados", s.installed, s.total)},
	}
	if s.configErr != nil {
		rows = append(rows, cardRow{colErr, "Origem", s.configErr.Error()})
	}
	return renderCard(rows, width)
}

func installPiConfig(dotfiles, home string, now time.Time) error {
	if err := validatePiConfig(dotfiles); err != nil {
		return err
	}
	files := piConfigFiles(dotfiles, home)
	timestamp := now.Format("20060102-150405")
	type change struct {
		file   piConfigFile
		backup string
	}
	var changes []change
	for _, file := range files {
		if regularFileMatches(file.source, file.destination) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(file.destination), 0755); err != nil {
			return fmt.Errorf("criar diretório de %s: %w", file.name, err)
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

func piAgentJobFor(dotfiles, home string, lookPath func(string) (string, error)) job {
	agentDir := piAgentDir(home)
	piPath, pathErr := lookPath("pi")
	steps := []step{nativeStep("Validar configuração do Pi", func() error {
		return validatePiConfig(dotfiles)
	})}
	if pathErr != nil {
		piPath = filepath.Join(agentDir, "bin", "pi")
		steps = append(steps, terminalStep("Instalar Pi Agent (não iniciar ao final)", "env", "PI_CODING_AGENT_DIR="+agentDir,
			"sh", "-c", "curl -fsSL https://pi.dev/install.sh | sh"))
	}
	steps = append(steps,
		nativeStep("Copiar settings e tema", func() error {
			return installPiConfig(dotfiles, home, time.Now())
		}),
		terminalStep("Instalar e atualizar pacotes do Pi", "env", "PI_CODING_AGENT_DIR="+agentDir,
			piPath, "update", "--extensions"),
	)
	return job{
		title: "Configurar Pi Agent",
		steps: steps,
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render(fmt.Sprintf("Pi Agent configurado com %d pacotes", len(requiredPiPackages)))
		},
	}
}

func piAgentJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	return piAgentJobFor(dotfiles, home, exec.LookPath)
}

func (m model) piAgentItems() []item {
	return []item{{
		title: "Configurar Pi Agent",
		desc:  "Copiar settings e tema, depois instalar todos os pacotes",
		job:   func() job { return piAgentJob(m.dotfiles) },
	}}
}
