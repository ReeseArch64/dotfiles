package main

import (
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

// Apenas leitura. O job também verifica chaves e política efetiva com sshd -T.
func laptopSSHRequirement(s sshStatus, home string) error {
	if len(s.missing) != 0 || s.partial || !s.service.isActive() || !s.service.isEnabled() || !s.listening || s.socket.isActive() || s.socket.isEnabled() {
		return fmt.Errorf("configure primeiro o SSH: OpenSSH e sshd.service ativo no boot, com configuração legível")
	}
	port, err := strconv.Atoi(s.port())
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("configure primeiro o SSH: porta inválida")
	}
	account, err := user.Current()
	if err != nil {
		return err
	}
	for key, expected := range map[string]string{
		"permitrootlogin": "no", "passwordauthentication": "no", "kbdinteractiveauthentication": "no",
		"pubkeyauthentication": "yes", "authenticationmethods": "publickey", "allowusers": account.Username,
		"authorizedkeysfile": ".ssh/authorized_keys", "authorizedkeyscommand": "none", "trustedusercakeys": "none",
	} {
		if s.cfg[key] != expected {
			return fmt.Errorf("configure primeiro o SSH: política %s pendente", key)
		}
	}
	directory, err := os.Lstat(filepath.Join(home, ".ssh"))
	if err != nil || !directory.IsDir() || directory.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("diretório SSH ausente ou com permissões inseguras")
	}
	for _, name := range []string{"id_laptop", "id_laptop.pub", "authorized_keys", "config"} {
		info, err := os.Lstat(filepath.Join(home, ".ssh", name))
		if err != nil || !info.Mode().IsRegular() || (name != "authorized_keys" && info.Size() == 0) {
			return fmt.Errorf("configure primeiro o SSH: %s ausente ou inválido", name)
		}
		if (name == "id_laptop" || name == "authorized_keys") && info.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("permissões inseguras: %s", name)
		}
	}
	client, err := os.ReadFile(filepath.Join(home, ".ssh", "config"))
	if err != nil || !strings.Contains(string(client), "IdentityFile ~/.ssh/id_laptop") {
		return fmt.Errorf("configure primeiro o cliente SSH com id_laptop")
	}
	contents, err := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(contents), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, _, valid := publicKeyID(line); !valid {
			return fmt.Errorf("chave autorizada inválida")
		}
	}
	return nil
}

func (m model) laptopSSHJob(action, title string) job {
	steps := []step{laptopSSHProcessStep(title, action)}
	steps = append(withSudo(ensurePkgs("openssh")...), steps...)
	return job{title: title, steps: steps}
}

func validateLaptopFirewall(port, lan, connection string) error {
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("porta SSH inválida: %q", port)
	}
	ip, subnet, err := net.ParseCIDR(lan)
	if err != nil || ip.To4() == nil || !ip.IsPrivate() || !subnet.IP.IsPrivate() {
		return fmt.Errorf("LAN IPv4 privada não detectada: %q; configure a rede antes de ativar o firewall", lan)
	}
	last := append(net.IP(nil), subnet.IP...)
	for i := range last {
		last[i] |= ^subnet.Mask[i]
	}
	if !last.IsPrivate() {
		return fmt.Errorf("LAN contém endereços públicos: %q", lan)
	}
	if connection != "" {
		fields := strings.Fields(connection)
		if len(fields) != 4 || net.ParseIP(fields[0]) == nil || net.ParseIP(fields[2]) == nil {
			return fmt.Errorf("SSH_CONNECTION inválida; firewall não foi alterado")
		}
		for _, p := range []string{fields[1], fields[3]} {
			n, err := strconv.Atoi(p)
			if err != nil || n < 1 || n > 65535 {
				return fmt.Errorf("porta em SSH_CONNECTION inválida")
			}
		}
	}
	return nil
}

func (m model) laptopItems() []item {
	switch m.screen {
	case screenWebBrowser:
		return []item{{title: "Firefox", desc: "Validar Mozilla, preparar pasta de restauração e definir padrão", job: laptopFirefoxJob}}
	case screenSSH:
		return []item{
			{title: "Configurar SSH", desc: "Gerar id_laptop e configurar acesso somente por chave", job: func() job { return m.laptopSSHJob("configure", "Configurar SSH") }},
			{title: "Chaves autorizadas", desc: "Adicionar, visualizar e remover chaves públicas", goTo: screenSSHKeys},
		}
	case screenSSHKeys:
		return []item{
			{title: "Adicionar chave pública", desc: "Colar chave pública de um dispositivo", job: func() job { return m.laptopSSHJob("add", "Adicionar chave pública") }},
			{title: "Visualizar chaves atuais", desc: "Mostrar chaves autorizadas e fingerprints", job: func() job { return m.laptopSSHJob("list", "Visualizar chaves atuais") }},
			{title: "Remover chave pública", desc: "Escolher e confirmar a remoção de uma chave", job: func() job { return m.laptopSSHJob("remove", "Remover chave pública") }},
		}
	}
	home, err := os.UserHomeDir()
	requirement := err
	if requirement == nil {
		requirement = laptopSSHRequirement(m.ssh, home)
	}
	switch m.screen {
	case screenMain:
		return []item{
			{title: "SSH", desc: "Configurar SSH e gerenciar chaves autorizadas", goTo: screenSSH},
			{title: "Firewall", desc: "Requer SSH configurado", goTo: screenFirewall, disabled: requirement != nil},
			{title: "Aparência", desc: "Copiar foto de perfil e wallpaper do laptop", job: func() job { return laptopAppearanceJob(m.dotfiles) }},
			{title: "Navegador Web", desc: "Configurar Firefox", goTo: screenWebBrowser},
			{title: "Sair", desc: "Até a próxima!", quit: true},
		}
	case screenFirewall:
		if requirement != nil {
			return []item{{title: "Configurar Firewall", desc: "Requer SSH configurado", disabled: true}}
		}
		return []item{{title: "Configurar Firewall", desc: "Ativar UFW e permitir SSH somente na LAN", job: func() job {
			steps := []step{laptopSSHProcessStep("Verificar pré-requisito SSH", "verify"),
				nativeStep("Revalidar porta SSH e LAN", func() error {
					home, err := os.UserHomeDir()
					if err != nil {
						return err
					}
					current := loadSSHStatusForDevice(deviceLaptop)
					if err := laptopSSHRequirement(current, home); err != nil {
						return err
					}
					if current.port() != m.fw.port || lanSubnet() != m.fw.lan {
						return fmt.Errorf("porta SSH ou LAN mudou; reabra a tela Firewall antes de configurar")
					}
					connection := os.Getenv("SSH_CONNECTION")
					if err := validateLaptopFirewall(m.fw.port, m.fw.lan, connection); err != nil {
						return err
					}
					if connection != "" {
						_, subnet, _ := net.ParseCIDR(m.fw.lan)
						if !subnet.Contains(net.ParseIP(strings.Fields(connection)[0])) {
							return fmt.Errorf("a conexão SSH atual está fora da LAN; configure o firewall localmente")
						}
					}
					return nil
				})}
			steps = append(steps, configureFirewallSteps(m.fw.port, m.fw.lan)...)
			return job{title: "Configurar Firewall", steps: withSudo(steps...)}
		}}}
	default:
		return nil
	}
}
