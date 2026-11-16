package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Apenas leitura: não instala nem configura SSH.
func laptopSSHRequirement(s sshStatus, home string) error {
	if len(s.missing) != 0 || s.partial || !s.service.isActive() || !s.service.isEnabled() {
		return fmt.Errorf("configure primeiro o SSH: OpenSSH e sshd.service ativo no boot, com configuração legível")
	}
	port, err := strconv.Atoi(s.port())
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("configure primeiro o SSH: porta inválida")
	}
	if missing := missingSSHIdentityFiles(home, requiredSSHIdentityFiles(deviceNotebook)); len(missing) != 0 {
		return fmt.Errorf("configure primeiro o SSH: chaves ausentes: %s", strings.Join(missing, ", "))
	}
	expected, err := os.ReadFile(filepath.Join(home, ".ssh", "id_trabalho_desktop.pub"))
	if err != nil {
		return fmt.Errorf("não foi possível verificar a chave do desktop: %w", err)
	}
	kind, key, valid := publicKeyID(string(expected))
	if !valid {
		return fmt.Errorf("chave pública do desktop inválida")
	}
	authorized, err := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	if err != nil {
		return fmt.Errorf("configure primeiro o SSH: authorized_keys indisponível")
	}
	count := 0
	for _, line := range strings.Split(string(authorized), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, value, ok := publicKeyID(line)
		if !ok || k != kind || value != key {
			return fmt.Errorf("authorized_keys deve autorizar somente a chave do desktop")
		}
		count++
	}
	if count != 1 {
		return fmt.Errorf("authorized_keys deve conter somente uma chave do desktop")
	}
	client, err := os.ReadFile(filepath.Join(home, ".ssh", "config"))
	if err != nil || len(strings.TrimSpace(string(client))) == 0 {
		return fmt.Errorf("configure primeiro o cliente SSH em ~/.ssh/config")
	}
	return nil
}

func (m model) laptopItems() []item {
	home, err := os.UserHomeDir()
	requirement := err
	if requirement == nil {
		requirement = laptopSSHRequirement(m.ssh, home)
	}
	switch m.screen {
	case screenMain:
		title := "Firewall"
		if requirement != nil {
			title = "🔒 Firewall"
		}
		return []item{
			{title: title, desc: "Requer SSH configurado como no notebook", goTo: screenFirewall},
			{title: "Sair", desc: "Até a próxima!", quit: true},
		}
	case screenFirewall:
		if requirement != nil {
			return []item{{title: "🔒 Configurar Firewall", desc: requirement.Error(), disabled: true}}
		}
		return []item{{title: "Configurar Firewall", desc: "Ativar UFW e permitir SSH somente na LAN", job: func() job {
			j := configureFirewallJob(m.fw.port, m.fw.lan, true)
			j.steps = append([]step{nativeStep("Verificar pré-requisito SSH", func() error {
				home, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				current := loadSSHStatusForDevice(deviceNotebook)
				if err := laptopSSHRequirement(current, home); err != nil {
					return err
				}
				if current.port() != m.fw.port || profileLANSubnet(deviceLaptop) != m.fw.lan {
					return fmt.Errorf("porta SSH ou LAN mudou; reabra a tela Firewall antes de configurar")
				}
				return nil
			})}, j.steps...)
			return j
		}}}
	default:
		return nil
	}
}
