package main

// Telas de SSH e Firewall: status lido nativamente e ações que viram jobs.

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var colWarn = lipgloss.Color("#FACC15")

type cardRow struct {
	color        lipgloss.Color
	label, value string
}

func renderCard(rows []cardRow, width int) string {
	label := lipgloss.NewStyle().Foreground(colDim).Width(10)
	val := lipgloss.NewStyle().Foreground(colTitle)
	var out []string
	for _, r := range rows {
		dot := "  "
		if r.color != "" {
			dot = lipgloss.NewStyle().Foreground(r.color).Render("●") + " "
		}
		out = append(out, dot+label.Render(r.label)+" "+val.Render(ansi.Truncate(r.value, width-13, "…")))
	}
	return strings.Join(out, "\n")
}

// ---- SSH ----

type sshStatus struct {
	missing         []string
	service, socket unitState
	cfg             map[string]string
	partial         bool
	keys            int
	addrs           []ifaceAddr
}

func loadSSHStatus() sshStatus {
	units := unitStates("sshd.service", "sshd.socket")
	cfg, partial := readSSHDConfig()
	return sshStatus{
		missing: missingPkgs("openssh"),
		service: units["sshd.service"],
		socket:  units["sshd.socket"],
		cfg:     cfg,
		partial: partial,
		keys:    authorizedKeys(),
		addrs:   lanAddrs(),
	}
}

func (s sshStatus) port() string { return s.cfg["port"] }

func (s sshStatus) card(width int) string {
	var rows []cardRow
	if len(s.missing) > 0 {
		rows = append(rows, cardRow{colErr, "Pacote", "openssh não instalado"})
	}
	switch {
	case s.service.isActive():
		rows = append(rows, cardRow{colOK, "Servidor", "rodando (sshd.service)"})
	case s.socket.isActive():
		rows = append(rows, cardRow{colOK, "Servidor", "escutando via sshd.socket"})
	default:
		rows = append(rows, cardRow{colDim, "Servidor", "parado"})
	}
	switch {
	case s.service.isEnabled():
		rows = append(rows, cardRow{colOK, "Boot", "sshd.service habilitado"})
	case s.socket.isEnabled():
		rows = append(rows, cardRow{colOK, "Boot", "sshd.socket habilitado"})
	default:
		rows = append(rows, cardRow{colDim, "Boot", "não inicia no boot"})
	}

	pw := s.cfg["passwordauthentication"]
	loginColor := colOK
	if pw != "no" {
		loginColor = colWarn
	}
	rows = append(rows, cardRow{loginColor, "Login", fmt.Sprintf("porta %s · senha: %s · root: %s", s.port(), yesNo(pw), s.cfg["permitrootlogin"])})

	keyColor := colOK
	if s.keys == 0 {
		keyColor = colWarn
	}
	rows = append(rows, cardRow{keyColor, "Chaves", fmt.Sprintf("%d em ~/.ssh/authorized_keys", s.keys)})

	var ips []string
	for _, a := range s.addrs {
		ips = append(ips, fmt.Sprintf("%s (%s)", a.net.IP, a.name))
	}
	if len(ips) == 0 {
		ips = []string{"nenhuma interface de rede ativa"}
	}
	rows = append(rows, cardRow{colDim, "Endereço", strings.Join(ips, ", ")})
	if s.partial {
		rows = append(rows, cardRow{colWarn, "Aviso", "parte do sshd_config só é legível como root"})
	}
	return renderCard(rows, width)
}

func yesNo(v string) string {
	if v == "no" {
		return "não"
	}
	return "sim"
}

func (m model) sshItems() []item {
	return []item{{
		title: "Configurar SSH",
		desc:  "Copia o cliente, libera a LAN e ativa o servidor no boot",
		job: func() job {
			return configureSSHJob(m.dotfiles, m.ssh.port(), lanSubnet())
		},
	}}
}

func configureSSHJob(dotfiles, port, lan string) job {
	home, _ := os.UserHomeDir()
	steps := []step{nativeStep("Validar chaves e copiar ~/.ssh/config", func() error {
		return installSSHClientConfig(dotfiles, home, time.Now())
	})}
	setup := ensurePkgs("openssh")
	setup = append(setup, configureFirewallSteps(port, lan)...)
	setup = append(setup,
		sudoStep("Desabilitar sshd.socket", true, "systemctl", "disable", "--now", "sshd.socket"),
		sudoStep("Habilitar e iniciar sshd.service", false, "systemctl", "enable", "--now", "sshd.service"))
	steps = append(steps, withSudo(setup...)...)
	return job{title: "Configurar SSH", steps: steps, result: sshResult}
}

// sshResult mostra como conectar e avisa se o firewall está barrando a porta.
func sshResult() string {
	s := loadSSHStatus()
	user := os.Getenv("USER")
	cmd := lipgloss.NewStyle().Foreground(colTitle).Bold(true)
	lines := []string{lipgloss.NewStyle().Foreground(colDim).Render("Conecte com:")}
	for _, a := range s.addrs {
		c := fmt.Sprintf("ssh %s@%s", user, a.net.IP)
		if s.port() != "22" {
			c += " -p " + s.port()
		}
		lines = append(lines, "  "+cmd.Render(c))
	}
	fw := loadFWStatus(s.port())
	if fw.active() && fw.rulesErr == nil && !fw.sshAllowed() {
		lines = append(lines, "", lipgloss.NewStyle().Foreground(colWarn).Render(
			"⚠ O firewall está ativo e não libera a porta "+s.port()+". Execute Configurar Firewall."))
	}
	return strings.Join(lines, "\n")
}

// ---- Firewall (UFW) ----

type fwStatus struct {
	missing   []string
	service   unitState
	enabled   bool
	policyIn  string
	policyOut string
	rules     []ufwRule
	rulesErr  error
	lan, port string
}

func loadFWStatus(port string) fwStatus {
	rules, err := readUFWRules()
	return fwStatus{
		missing:   missingPkgs("ufw", "iptables-nft"),
		service:   unitStates("ufw.service")["ufw.service"],
		enabled:   readKV("/etc/ufw/ufw.conf", "ENABLED") == "yes",
		policyIn:  readKV("/etc/default/ufw", "DEFAULT_INPUT_POLICY"),
		policyOut: readKV("/etc/default/ufw", "DEFAULT_OUTPUT_POLICY"),
		rules:     rules,
		rulesErr:  err,
		lan:       lanSubnet(),
		port:      port,
	}
}

func (f fwStatus) installed() bool { return !slices.Contains(f.missing, "ufw") }
func (f fwStatus) active() bool    { return f.enabled && f.service.isActive() }

func (f fwStatus) sshAllowed() bool {
	for _, r := range f.rules {
		if r.allowsPort(f.port) {
			return true
		}
	}
	return false
}

// v4Rules deduplica: regras de qualquer origem aparecem em user.rules e user6.rules.
func (f fwStatus) v4Rules() []ufwRule {
	var out []ufwRule
	for _, r := range f.rules {
		if r.v6 && r.anySource() {
			continue
		}
		out = append(out, r)
	}
	return out
}

func (f fwStatus) card(width int) string {
	var rows []cardRow
	if !f.installed() {
		return renderCard([]cardRow{{colErr, "Pacote", "ufw não instalado"}, {colDim, "LAN", f.lan}}, width)
	}
	switch {
	case f.active():
		rows = append(rows, cardRow{colOK, "Firewall", "ativo"})
	case f.enabled:
		rows = append(rows, cardRow{colWarn, "Firewall", "ativado, mas ufw.service está parado"})
	default:
		rows = append(rows, cardRow{colWarn, "Firewall", "inativo"})
	}
	rows = append(rows, cardRow{colDim, "Política", fmt.Sprintf("entrada %s · saída %s", strings.ToLower(f.policyIn), strings.ToLower(f.policyOut))})

	switch {
	case f.rulesErr != nil:
		rows = append(rows, cardRow{colWarn, "Regras", "sem permissão para ler /etc/ufw/user.rules"})
	case f.sshAllowed():
		rows = append(rows, cardRow{colOK, "SSH", "porta " + f.port + " liberada"})
	default:
		rows = append(rows, cardRow{colWarn, "SSH", "porta " + f.port + " fechada"})
	}

	rules := f.v4Rules()
	if f.rulesErr == nil && len(rules) == 0 {
		rows = append(rows, cardRow{colDim, "Regras", "nenhuma"})
	}
	for i, r := range rules {
		label := ""
		if i == 0 {
			label = "Regras"
		}
		if i == 4 && len(rules) > 5 {
			rows = append(rows, cardRow{"", label, fmt.Sprintf("… e mais %d", len(rules)-4)})
			break
		}
		rows = append(rows, cardRow{colDim, label, describeRule(r)})
	}
	rows = append(rows, cardRow{colDim, "LAN", f.lan})
	return renderCard(rows, width)
}

func describeRule(r ufwRule) string {
	src := r.src
	if r.anySource() {
		src = "qualquer origem"
	}
	s := fmt.Sprintf("%s %s", r.action, r.dport)
	if r.proto != "any" {
		s += "/" + r.proto
	}
	s += " ← " + src
	if r.comment != "" {
		s += " · " + r.comment
	}
	return s
}

func (m model) fwItems() []item {
	return []item{{
		title: "Configurar Firewall",
		desc:  fmt.Sprintf("Ativa o UFW e libera SSH na LAN %s", m.fw.lan),
		job: func() job {
			return configureFirewallJob(m.fw.port, m.fw.lan)
		},
	}}
}

func configureFirewallSteps(port, lan string) []step {
	steps := ensurePkgs("ufw", "iptables-nft")
	return append(steps,
		sudoStep("Entrada: bloquear por padrão", false, "ufw", "default", "deny", "incoming"),
		sudoStep("Saída: permitir por padrão", false, "ufw", "default", "allow", "outgoing"),
		sudoStep("Remover liberação global de "+port+"/tcp", true, "ufw", "--force", "delete", "allow", port+"/tcp"),
		sudoStep("Liberar "+port+"/tcp para "+lan, false,
			"ufw", "allow", "from", lan, "to", "any", "port", port, "proto", "tcp", "comment", "SSH LAN"),
		sudoStep("Habilitar ufw.service no boot", false, "systemctl", "enable", "--now", "ufw.service"),
		sudoStep("Ativar UFW", false, "ufw", "--force", "enable"))
}

func configureFirewallJob(port, lan string) job {
	return job{
		title: "Configurar Firewall",
		steps: withSudo(configureFirewallSteps(port, lan)...),
	}
}
