package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var dockerPackages = []string{"docker", "docker-compose", "lazydocker", "docker-buildx"}
var dockerDependencies = []string{"util-linux", "xdg-utils"}

func dockerRequiredPackages() []string {
	return append(append([]string{}, dockerPackages...), dockerDependencies...)
}

type dockerStatus struct {
	missing  []string
	service  unitState
	username string
	inGroup  bool
}

func currentUsername() (string, error) {
	current, err := user.Current()
	if err == nil && current.Username != "" {
		return current.Username, nil
	}
	if username := os.Getenv("USER"); username != "" {
		return username, nil
	}
	return "", errors.New("não foi possível identificar o usuário atual")
}

func userInGroup(username, groupName string) bool {
	account, err := user.Lookup(username)
	if err != nil {
		return false
	}
	group, err := user.LookupGroup(groupName)
	if err != nil {
		return false
	}
	ids, err := account.GroupIds()
	if err != nil {
		return false
	}
	for _, id := range ids {
		if id == group.Gid {
			return true
		}
	}
	return false
}

func loadDockerStatus() dockerStatus {
	username, _ := currentUsername()
	return dockerStatus{
		missing:  missingPkgs(dockerPackages...),
		service:  unitStates("docker.service")["docker.service"],
		username: username,
		inGroup:  username != "" && userInGroup(username, "docker"),
	}
}

func (s dockerStatus) card(width int) string {
	missing := make(map[string]bool, len(s.missing))
	for _, name := range s.missing {
		missing[name] = true
	}
	rows := make([]cardRow, 0, len(dockerPackages)+3)
	for _, name := range dockerPackages {
		color, value := colOK, name+" instalado"
		if missing[name] {
			color, value = colErr, name+" não instalado"
		}
		rows = append(rows, cardRow{color, "Pacote", value})
	}
	serviceColor, serviceValue := colOK, "docker.service rodando"
	if !s.service.isActive() {
		serviceColor, serviceValue = colWarn, "docker.service parado"
	}
	rows = append(rows, cardRow{serviceColor, "Serviço", serviceValue})
	bootColor, bootValue := colOK, "habilitado no boot"
	if !s.service.isEnabled() {
		bootColor, bootValue = colWarn, "não habilitado no boot"
	}
	rows = append(rows, cardRow{bootColor, "Boot", bootValue})
	groupUser := s.username
	if groupUser == "" {
		groupUser = "usuário atual"
	}
	groupColor, groupValue := colOK, groupUser+" pertence ao grupo docker"
	if !s.inGroup {
		groupColor, groupValue = colWarn, groupUser+" não pertence ao grupo docker"
	}
	rows = append(rows, cardRow{groupColor, "Grupo", groupValue})
	return renderCard(rows, width)
}

func validateDockerLoginBrowser(home string, lookPath func(string) (string, error)) error {
	var missing []string
	if _, err := lookPath("zen-browser"); err != nil {
		missing = append(missing, "executável zen-browser")
	}
	config := filepath.Join(home, ".config", "zen")
	if info, err := os.Stat(config); err != nil || !info.IsDir() {
		missing = append(missing, config)
	}
	if len(missing) > 0 {
		return errors.New("Docker login exige Zen Browser configurado; ausente: " + strings.Join(missing, ", "))
	}
	return nil
}

func dockerHasCredentials(home string) bool {
	content, err := os.ReadFile(filepath.Join(home, ".docker", "config.json"))
	if err != nil {
		return false
	}
	var config struct {
		Auths map[string]struct {
			Auth          string `json:"auth"`
			IdentityToken string `json:"identitytoken"`
		} `json:"auths"`
		CredsStore  string            `json:"credsStore"`
		CredHelpers map[string]string `json:"credHelpers"`
	}
	if json.Unmarshal(content, &config) != nil {
		return false
	}
	for registry, auth := range config.Auths {
		if (registry == "https://index.docker.io/v1/" || registry == "docker.io" || registry == "https://registry-1.docker.io") && (auth.Auth != "" || auth.IdentityToken != "") {
			return true
		}
	}
	stores := map[string]bool{}
	if config.CredsStore != "" {
		stores[config.CredsStore] = true
	}
	for _, store := range config.CredHelpers {
		stores[store] = true
	}
	for store := range stores {
		out, err := exec.Command("docker-credential-"+store, "list").Output()
		if err == nil {
			var credentials map[string]string
			if json.Unmarshal(out, &credentials) == nil {
				for registry := range credentials {
					if registry == "https://index.docker.io/v1/" || registry == "docker.io" || registry == "https://registry-1.docker.io" {
						return true
					}
				}
			}
		}
	}
	return false
}

func dockerJobForUser(username, home string, packageSteps []step, lookPath func(string) (string, error)) job {
	steps := []step{skipWhen(nativeStep("Validar Zen Browser para Docker login", func() error {
		return validateDockerLoginBrowser(home, lookPath)
	}), func() bool { return dockerHasCredentials(home) })}
	steps = append(steps, packageSteps...)
	steps = append(steps,
		terminalStep("Adicionar "+username+" ao grupo docker", "sudo", "usermod", "-aG", "docker", username),
		terminalStep("Habilitar e iniciar docker.service", "sudo", "systemctl", "enable", "--now", "docker.service"),
		terminalStep("Definir Zen Browser como padrão", "xdg-settings", "set", "default-web-browser", "zen.desktop"),
		skipWhen(terminalStep("Autenticar no Docker", "newgrp", "docker", "-c", "docker login"), func() bool { return dockerHasCredentials(home) }),
	)
	return job{
		title: "Configurar Docker",
		steps: steps,
		result: func() string {
			return strings.Join([]string{
				lipgloss.NewStyle().Foreground(colOK).Render("Docker configurado e autenticação concluída."),
				lipgloss.NewStyle().Foreground(colWarn).Render("Abra uma nova sessão para aplicar o grupo docker aos outros terminais."),
			}, "\n")
		},
	}
}

func dockerJob() job {
	username, err := currentUsername()
	if err != nil {
		return job{title: "Configurar Docker", steps: []step{nativeStep("Identificar usuário atual", func() error { return err })}}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return job{title: "Configurar Docker", steps: []step{nativeStep("Identificar diretório pessoal", func() error { return err })}}
	}
	return dockerJobForUser(username, home, ensurePkgs(dockerRequiredPackages()...), exec.LookPath)
}

func (m model) dockerItems() []item {
	return []item{{
		title: "Configurar Docker",
		desc:  "Instalar pacotes, adicionar ao grupo, ativar serviço e executar docker login",
		job:   dockerJob,
	}}
}
