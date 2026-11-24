package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func setupNoctalia(p profile, output io.Writer, commands sshCommands) error {
	if p.sshKeyName() == "" || os.Geteuid() == 0 {
		return errors.New("configure o Noctalia pelo menu Desktop ou Laptop como usuário normal")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(home) {
		return errors.New("a pasta pessoal precisa ter um caminho absoluto")
	}
	directory := noctaliaDirectory(home)
	source := dotfilesAsset(home, "cachyos", "noctalia")
	backup, err := configureNoctalia(source, directory, home, managedBackup("Noctalia", commands.interactive))
	if backup != "" {
		fmt.Fprintln(output, "Backup:", backup)
	}
	if err == nil {
		fmt.Fprintln(output, "Configuração aplicada em", directory)
	}
	return err
}

func noctaliaDirectory(home string) string {
	state := filepath.Join(home, ".local", "state")
	for _, key := range []string{"NOCTALIA_STATE_HOME", "XDG_STATE_HOME"} {
		if path := os.Getenv(key); filepath.IsAbs(path) {
			state = path
			break
		}
	}
	return filepath.Join(state, "noctalia")
}

var noctaliaPlugins = []string{
	"community/androidconnect",
	"community/github-activity",
	"community/godot-provider",
	"community/kuma-status",
	"community/kumacat",
	"community/pomodoro-heatmap",
	"community/process-manager",
	"community/proton-pass",
	"community/shipyard",
	"community/ufw",
	"community/zed-provider",
	"official/world_clock",
}

func missingNoctaliaPlugins(directory string) []string {
	var missing []string
	for _, path := range noctaliaPlugins {
		info, err := os.Stat(filepath.Join(directory, "plugins", "materialized", path))
		if err != nil || !info.IsDir() {
			missing = append(missing, path)
		}
	}
	return missing
}

func showNoctaliaRequirements(_ profile, c sshConsole) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	missing := missingNoctaliaPlugins(noctaliaDirectory(home))
	if len(missing) == 0 {
		fmt.Fprint(c.output, "Todos os plugins exigidos estão instalados.\n\n")
	} else {
		fmt.Fprint(c.output, "Instale os plugins pendentes no Noctalia antes de aplicar a configuração.\n\n")
	}
	for _, plugin := range noctaliaPlugins {
		state := "Instalado"
		if slices.Contains(missing, plugin) {
			state = "Pendente"
		}
		fmt.Fprintf(c.output, "%s · %s\n", state, plugin)
	}
	return nil
}

func configureNoctalia(source, directory, home string, location backupLocation) (string, error) {
	unlock, err := lockOperation(directory, "configuração do Noctalia")
	if err != nil {
		return "", err
	}
	defer unlock()
	if missing := missingNoctaliaPlugins(directory); len(missing) != 0 {
		return "", errors.New("Instale os plugins abaixo no Noctalia e execute esta ação novamente:\n\n• " + strings.Join(missing, "\n• "))
	}
	quotedHome, err := json.Marshal(home)
	if err != nil {
		return "", err
	}
	var files []configFile
	for _, name := range []string{"settings.toml", "state.toml"} {
		contents, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			return "", fmt.Errorf("ler o arquivo %s do Noctalia: %w", name, err)
		}
		rendered := strings.ReplaceAll(string(contents), "@DOTFILES_HOME@", string(quotedHome[1:len(quotedHome)-1]))
		files = append(files, configFile{filepath.Join(directory, name), []byte(rendered), 0600})
	}
	return applyConfigFiles(files, location, "noctalia")
}
