package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func runNoctaliaSetup(p profile) error {
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
	state := filepath.Join(home, ".local", "state")
	for _, key := range []string{"NOCTALIA_STATE_HOME", "XDG_STATE_HOME"} {
		if path := os.Getenv(key); filepath.IsAbs(path) {
			state = path
			break
		}
	}
	directory := filepath.Join(state, "noctalia")
	source := dotfilesAsset(home, "cachyos", "noctalia")
	console := sshConsole{home: home, input: bufio.NewReader(os.Stdin), output: os.Stdout, commands: systemSSHCommands()}
	location := managedBackup("Noctalia", console.commands.interactive)
	printActionHeader()
	fmt.Fprintln(console.output, "Aplicar settings.toml e state.toml em", directory)
	fmt.Fprintln(console.output, "Arquivos substituídos recebem backup em", location.path)
	if err = console.confirm("Digite CONFIRMO para configurar: ", "CONFIRMO"); err == nil {
		var backup string
		backup, err = configureNoctalia(source, directory, home, location)
		if backup != "" {
			fmt.Fprintln(console.output, "Backup:", backup)
		}
	}
	if err == nil {
		fmt.Fprintln(console.output, "Noctalia configurado.")
	} else {
		fmt.Fprintln(console.output, err)
	}
	_, _ = console.prompt("\nEnter para voltar ao menu...")
	return err
}

func configureNoctalia(source, directory, home string, location backupLocation) (string, error) {
	unlock, err := lockOperation(directory, "configuração do Noctalia")
	if err != nil {
		return "", err
	}
	defer unlock()
	plugins := filepath.Join(directory, "plugins", "materialized", "community")
	var missing []string
	for _, name := range []string{"containers", "github-kanban", "zed-provider"} {
		info, err := os.Stat(filepath.Join(plugins, name))
		if err != nil || !info.IsDir() {
			missing = append(missing, name)
		}
	}
	if len(missing) != 0 {
		return "", errors.New("baixe os plugins ausentes no Noctalia: " + strings.Join(missing, ", "))
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
