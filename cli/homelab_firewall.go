package main

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

type homelabConnection struct {
	client, server, port string
}

func parseHomelabConnection(connection string) (homelabConnection, error) {
	if connection == "" {
		return homelabConnection{}, nil
	}
	fields := strings.Fields(connection)
	if len(fields) != 4 {
		return homelabConnection{}, errors.New("SSH_CONNECTION inválida")
	}
	client, server := net.ParseIP(fields[0]), net.ParseIP(fields[2])
	if client == nil || server == nil {
		return homelabConnection{}, errors.New("endereço inválido em SSH_CONNECTION")
	}
	for _, port := range []string{fields[1], fields[3]} {
		if n, err := strconv.ParseUint(port, 10, 16); err != nil || n == 0 {
			return homelabConnection{}, errors.New("porta inválida em SSH_CONNECTION")
		}
	}
	return homelabConnection{client.String(), server.String(), fields[3]}, nil
}

func configureHomelabFirewall(c *sshConsole) error {
	lan, err := detectLAN()
	if err != nil {
		return err
	}
	fmt.Fprintf(c.output, "O UFW permitirá entrada em todas as portas e protocolos pela LAN %s (%s). As regras existentes e o acesso da sessão SSH atual serão preservados.\n", lan.subnet, lan.device)
	if err := c.confirm("Ativar o firewall com a política de acesso pela LAN?"); err != nil {
		return err
	}
	if err := c.ensureUbuntuPackages("ufw"); err != nil {
		return err
	}
	return c.homelabPrivileged("apply-firewall")
}

func (f firewall) applyHomelab(lan localNetwork, connection string) error {
	if lan.device == "" || lan.subnet == nil || !privateIPv4Subnet(lan.subnet) {
		return errors.New("LAN IPv4 privada inválida para o UFW")
	}
	remote, err := parseHomelabConnection(connection)
	if err != nil {
		return err
	}
	files, err := f.snapshot()
	if err != nil {
		return err
	}
	if remote.client != "" && net.ParseIP(remote.client).To4() == nil && ufwSetting(files[0].data, "IPV6") != "yes" {
		return errors.New("habilite IPV6=yes em /etc/default/ufw para preservar a sessão SSH atual")
	}
	commands := [][]string{{"ufw", "prepend", "allow", "in", "on", lan.device, "from", lan.subnet.String(), "to", "any", "comment", "Homelab LAN"}}
	if remote.client != "" {
		commands = append(commands, []string{"ufw", "prepend", "allow", "from", remote.client, "to", "any", "port", remote.port, "proto", "tcp", "comment", "SSH admin atual"})
	}
	commands = append(commands,
		[]string{"ufw", "default", "deny", "incoming"},
		[]string{"ufw", "default", "allow", "outgoing"},
		[]string{"ufw", "--force", "enable"},
		[]string{"systemctl", "enable", "--now", "ufw.service"},
		[]string{"systemctl", "is-active", "--quiet", "ufw.service"},
	)
	return f.applyWithRollback(files, func() error {
		for _, command := range commands {
			if _, err := f.commands.capture("", command...); err != nil {
				return err
			}
		}
		return nil
	})
}
