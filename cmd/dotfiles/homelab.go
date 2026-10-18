package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

func (m model) serverItems() []item {
	switch m.screen {
	case screenMain:
		return []item{
			{title: "Sistema", desc: "SSH e firewall para Ubuntu", goTo: screenSystem},
			{title: "CasaOS", desc: "Instalar usando https://get.casaos.io (sudo)", job: casaOSJob},
			{title: "Liberar porta 53 (DNS)", desc: "Desativar o stub do systemd-resolved preservando a resolução", job: func() job { return serverDNSJob(m.setupDir) }},
			{title: "Sair", desc: "Até a próxima!", quit: true},
		}
	case screenSystem:
		return []item{
			{title: "SSH", desc: "Ativar servidor mantendo a configuração e o acesso atuais", goTo: screenSSH},
			{title: "Firewall", desc: "UFW: SSH e CasaOS (80/tcp) na LAN; preservar regras existentes", goTo: screenFirewall},
		}
	case screenSSH:
		return []item{{title: "Configurar SSH", desc: "Instalar OpenSSH, validar e ativar ssh.service ou ssh.socket", job: func() job {
			return serverSSHJob(m.setupDir)
		}}}
	case screenFirewall:
		return []item{{title: "Configurar Firewall", desc: "Liberar SSH e CasaOS na LAN " + m.fw.lan + " antes de ativar o UFW", job: func() job {
			return serverFirewallJob(m.setupDir, m.fw.port, m.fw.lan, os.Getenv("SSH_CONNECTION"))
		}}}
	}
	return nil
}

func missingDebianPkgs(path string, names ...string) []string {
	content, _ := os.ReadFile(path)
	have := map[string]bool{}
	for _, paragraph := range strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n\n") {
		var name, status string
		for _, line := range strings.Split(paragraph, "\n") {
			key, value, _ := strings.Cut(line, ":")
			switch key {
			case "Package":
				name = strings.TrimSpace(value)
			case "Status":
				status = strings.TrimSpace(value)
			}
		}
		fields := strings.Fields(status)
		if len(fields) == 3 && fields[1] == "ok" && fields[2] == "installed" {
			have[name] = true
		}
	}
	var missing []string
	for _, name := range names {
		if !have[name] {
			missing = append(missing, name)
		}
	}
	return missing
}

func sshMissingPkgs(device deviceProfile) []string {
	if device == deviceServer {
		return missingDebianPkgs("/var/lib/dpkg/status", "openssh-server", "openssh-client")
	}
	return missingPkgs("openssh")
}

func firewallMissingPkgs(device deviceProfile) []string {
	if device == deviceServer {
		return missingDebianPkgs("/var/lib/dpkg/status", "ufw")
	}
	return missingPkgs("ufw", "iptables-nft")
}

func profileLANSubnet(device deviceProfile) string {
	if device != deviceServer {
		return lanSubnet()
	}
	for _, addr := range lanAddrs() {
		if addr.net.IP.IsPrivate() {
			return (&net.IPNet{IP: addr.net.IP.Mask(addr.net.Mask), Mask: addr.net.Mask}).String()
		}
	}
	return ""
}

func aptSteps(packages ...string) []step {
	installed := func() bool { return len(missingDebianPkgs("/var/lib/dpkg/status", packages...)) == 0 }
	return []step{
		skipWhen(terminalStep("Atualizar índices do APT", "sudo", "apt-get", "update"), installed),
		skipWhen(terminalStep("Instalar "+strings.Join(packages, ", "), append([]string{"sudo", "apt-get", "install", "--yes"}, packages...)...), installed),
	}
}

func serverSSHJob(setupDir string) job {
	steps := aptSteps("openssh-server", "openssh-client")
	steps = append(steps, sudoStep("Validar SSH e ativar serviço sem alterar autenticação", false,
		"bash", configPath(setupDir, "ssh", "enable.sh")))
	return job{title: "Configurar SSH (Ubuntu)", steps: withSudo(steps...), result: func() string { return sshResultForDevice(deviceServer) }}
}

func validateServerFirewall(port, lan, connection string) error {
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

func serverFirewallJob(setupDir, port, lan, connection string) job {
	steps := []step{nativeStep("Validar LAN e conexão administrativa", func() error { return validateServerFirewall(port, lan, connection) })}
	steps = append(steps, aptSteps("ufw")...)
	steps = append(steps, sudoStep("Liberar SSH e CasaOS na LAN, preservar acesso e ativar UFW", false,
		"bash", configPath(setupDir, "firewall", "configure.sh"), port, lan, connection))
	return job{title: "Configurar Firewall (Ubuntu)", steps: withSudo(steps...)}
}

const casaOSInstallScript = "set -o pipefail\ncurl -fsSL https://get.casaos.io | sudo bash"

func casaOSJob() job {
	steps := aptSteps("curl", "ca-certificates")
	steps = append(steps, terminalStep("Instalar CasaOS via curl (sudo)", "bash", "-c", casaOSInstallScript))
	return job{title: "Instalar CasaOS", steps: withSudo(steps...), result: func() string {
		return "CasaOS: acesse http://<IP-do-servidor> na LAN (porta 80). DNS/porta 53: ação independente no menu."
	}}
}

func serverDNSJob(setupDir string) job {
	return job{title: "Liberar porta 53 (DNS)", steps: withSudo(sudoStep(
		"Desativar DNSStubListener, preservar resolv.conf e reiniciar systemd-resolved", false,
		"bash", configPath(setupDir, "dns", "disable-stub.sh"),
		"/etc/systemd/resolved.conf", "/etc/resolv.conf", "/run/systemd/resolve/stub-resolv.conf", "/run/systemd/resolve/resolv.conf")),
		result: func() string {
			return "DNSStubListener=no; systemd-resolved reiniciado. Nenhuma regra pública para a porta 53 foi criada."
		}}
}
