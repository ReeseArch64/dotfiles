package main

import (
	"errors"
	"os"
	"os/user"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var dockerPackages = []string{"docker", "docker-compose", "lazydocker", "docker-buildx", "kind"}
var dockerDependencies = []string{"util-linux"}

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

func dockerJobForUser(username string, packageSteps []step) job {
	steps := append([]step{}, packageSteps...)
	steps = append(steps,
		terminalStep("Adicionar "+username+" ao grupo docker", "sudo", "usermod", "-aG", "docker", username),
		terminalStep("Habilitar e iniciar docker.service", "sudo", "systemctl", "enable", "--now", "docker.service"),
		terminalStep("Autenticar no Docker", "newgrp", "docker", "-c", "docker login"),
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
	return dockerJobForUser(username, ensurePkgs(dockerRequiredPackages()...))
}

func (m model) dockerItems() []item {
	return []item{{
		title: "Configurar Docker",
		desc:  "Instalar pacotes, adicionar ao grupo, ativar serviço e executar docker login",
		job:   dockerJob,
	}}
}
