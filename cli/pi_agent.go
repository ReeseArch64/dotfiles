package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func runPiAgentSetup(device profile, console sshConsole) error {
	if device.sshKeyName() == "" || os.Geteuid() == 0 {
		return errors.New("configure o Pi Agent pelo menu Desktop ou Laptop como usuário normal")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	agentDir := os.Getenv("PI_CODING_AGENT_DIR")
	if agentDir == "" {
		agentDir = filepath.Join(home, ".pi", "agent")
	}
	if !filepath.IsAbs(home) || !filepath.IsAbs(agentDir) {
		return errors.New("as pastas pessoal e do Pi Agent precisam ter caminhos absolutos")
	}
	console.home = home
	location := managedBackup("Pi", console.commands.interactive)
	fmt.Fprintln(console.output, "Aplicar settings.json em", agentDir)
	fmt.Fprintln(console.output, "Preparar o Pi pelo Mise e instalar ou atualizar as extensões do modelo.")
	fmt.Fprintln(console.output, "Arquivos substituídos recebem backup em", location.path)
	return configurePiAgent(console, cachyosAsset(home, "pi", "agent/settings.json"), agentDir, location, exec.LookPath)
}

func configurePiAgent(console sshConsole, source, agentDir string, location backupLocation, lookup func(string) (string, error)) error {
	unlock, err := lockOperation(filepath.Join(console.home, "pi-agent"), "configuração do Pi Agent")
	if err != nil {
		return err
	}
	defer unlock()
	destination := filepath.Join(agentDir, "settings.json")
	settings, err := piAgentSettings(source, destination)
	if err != nil {
		return err
	}
	mise, err := ensureMise(&console, lookup, "git")
	if err != nil {
		return fmt.Errorf("instalar dependências do Pi Agent: %w", err)
	}
	command := []string{mise, "--cd", "/", "exec", "pi", "bun", "node", "--", "env", "PI_CODING_AGENT_DIR=" + agentDir, "pi"}
	if err := console.commands.interactive(append(command, "--version")...); err != nil {
		return fmt.Errorf("preparar o Pi com o Mise: %w", err)
	}
	backup, err := applyConfigFiles([]configFile{{destination, settings, 0600}}, location, "pi")
	if backup != "" {
		fmt.Fprintln(console.output, "Backup do Pi:", backup)
	}
	if err != nil {
		return err
	}
	if err := console.commands.interactive(append(command, "update", "--extensions")...); err != nil {
		return fmt.Errorf("configuração do Pi aplicada, mas a atualização de extensões falhou: %w", err)
	}
	return nil
}

func piAgentSettings(source, destination string) ([]byte, error) {
	data, err := os.ReadFile(source)
	if err != nil {
		return nil, fmt.Errorf("ler configuração do Pi: %w", err)
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("validar configuração do Pi: %w", err)
	}
	var packages []string
	if err := json.Unmarshal(settings["packages"], &packages); err != nil || len(packages) == 0 {
		return nil, errors.New("a configuração do Pi deve declarar os pacotes de extensões")
	}
	_, rewrite := settings["theme"]
	delete(settings, "theme")
	if info, err := os.Stat(destination); err == nil && info.Mode().IsRegular() {
		var current struct {
			Theme json.RawMessage `json:"theme"`
		}
		if original, err := os.ReadFile(destination); err == nil && json.Unmarshal(original, &current) == nil && current.Theme != nil {
			settings["theme"] = current.Theme
			rewrite = true
		}
	}
	if !rewrite {
		return data, nil
	}
	output, err := json.MarshalIndent(settings, "", "  ")
	return append(output, '\n'), err
}
