package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func runPiAgentSetup(device profile) error {
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
	console := sshConsole{home: home, input: bufio.NewReader(os.Stdin), output: os.Stdout, commands: systemSSHCommands()}
	location := managedBackup("Pi", console.commands.interactive)
	printActionHeader()
	fmt.Fprintln(console.output, "Aplicar settings.json e o tema Noctalia em", agentDir)
	fmt.Fprintln(console.output, "Preparar o Pi pelo Mise e instalar ou atualizar as extensões do modelo.")
	fmt.Fprintln(console.output, "Arquivos substituídos recebem backup em", location.path)
	err = console.confirm("Digite CONFIRMO para configurar o Pi Agent: ", "CONFIRMO")
	if err == nil {
		err = configurePiAgent(console, cachyosAsset(home, "pi", "agent"), agentDir, location)
	}
	if err != nil {
		fmt.Fprintln(console.output, err)
	} else {
		fmt.Fprintln(console.output, "Pi Agent configurado.")
	}
	_, _ = console.prompt("\nEnter para voltar ao menu...")
	return err
}

func configurePiAgent(console sshConsole, source, agentDir string, location backupLocation) error {
	files, err := piAgentConfigFiles(source, agentDir)
	if err != nil {
		return err
	}
	unlock, err := lockOperation(filepath.Join(console.home, ".dotfiles-pi.lock"), "configuração do Pi Agent")
	if err != nil {
		return err
	}
	defer unlock()
	if err := console.ensurePackages("mise", "git"); err != nil {
		return fmt.Errorf("instalar dependências do Pi Agent: %w", err)
	}
	command := []string{"mise", "--cd", "/", "exec", "pi", "bun", "node", "--", "env", "PI_CODING_AGENT_DIR=" + agentDir, "pi"}
	if err := console.commands.interactive(append(command, "--version")...); err != nil {
		return fmt.Errorf("preparar o Pi com o Mise: %w", err)
	}
	backup, err := applyConfigFiles(files, location, "pi")
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

func piAgentConfigFiles(source, agentDir string) ([]configFile, error) {
	const themeName = "noctalia"
	var settings struct {
		Theme    string   `json:"theme"`
		Packages []string `json:"packages"`
	}
	var theme struct {
		Name   string            `json:"name"`
		Colors map[string]string `json:"colors"`
	}
	var files []configFile
	for _, file := range []struct {
		name string
		mode os.FileMode
		data any
	}{
		{"settings.json", 0600, &settings},
		{filepath.Join("themes", themeName+".json"), 0644, &theme},
	} {
		data, err := os.ReadFile(filepath.Join(source, file.name))
		if err != nil {
			return nil, fmt.Errorf("ler configuração do Pi %s: %w", file.name, err)
		}
		if err := json.Unmarshal(data, file.data); err != nil {
			return nil, fmt.Errorf("validar configuração do Pi %s: %w", file.name, err)
		}
		files = append(files, configFile{filepath.Join(agentDir, file.name), data, file.mode})
	}
	if settings.Theme != themeName || theme.Name != themeName || len(theme.Colors) == 0 || len(settings.Packages) == 0 {
		return nil, errors.New("a configuração do Pi deve declarar o tema Noctalia e os pacotes de extensões")
	}
	return files, nil
}
