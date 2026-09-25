package main

// Telas de SSH e Firewall: status lido nativamente e ações que viram jobs.

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

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
	hardened        bool
	keys            int
	addrs           []ifaceAddr
}

func loadSSHStatus() sshStatus {
	units := unitStates("sshd.service", "sshd.socket")
	cfg, partial := readSSHDConfig()
	return sshStatus{
		missing:  missingPkgs("openssh"),
		service:  units["sshd.service"],
		socket:   units["sshd.socket"],
		cfg:      cfg,
		partial:  partial,
		hardened: fileExists(hardeningFile),
		keys:     authorizedKeys(),
		addrs:    lanAddrs(),
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
	s := m.ssh
	enable := func(title, unit, other string) job {
		steps := ensurePkgs("openssh")
		steps = append(steps,
			sudoStep("Desabilitar "+other, true, "systemctl", "disable", "--now", other),
			sudoStep("Habilitar e iniciar "+unit, false, "systemctl", "enable", "--now", unit))
		return job{title: title, steps: withSudo(steps...), result: sshResult}
	}

	its := []item{
		{title: "Ativar permanente", desc: "sshd.service sempre rodando e habilitado no boot",
			job: func() job { return enable("Ativar SSH permanente", "sshd.service", "sshd.socket") }},
		{title: "Ativar via socket", desc: "sshd.socket: o daemon só sobe quando chega uma conexão",
			job: func() job { return enable("Ativar SSH via socket", "sshd.socket", "sshd.service") }},
		{title: "Ativar temporário", desc: "Só para esta sessão: não persiste após reboot",
			job: func() job {
				steps := append(ensurePkgs("openssh"), sudoStep("Iniciar sshd.service", false, "systemctl", "start", "sshd.service"))
				return job{title: "Ativar SSH temporário", steps: withSudo(steps...), result: sshResult}
			}},
		{title: "Parar SSH", desc: "Para e desabilita sshd.service e sshd.socket",
			job: func() job {
				return job{title: "Parar SSH", steps: withSudo(
					sudoStep("Parar e desabilitar sshd.socket", true, "systemctl", "disable", "--now", "sshd.socket"),
					sudoStep("Parar e desabilitar sshd.service", true, "systemctl", "disable", "--now", "sshd.service"),
				)}
			}},
	}

	if s.hardened {
		its = append(its, item{title: "Remover hardening", desc: "Volta a permitir login por senha (remove 10-hardening.conf)",
			job: func() job {
				return job{title: "Remover hardening", steps: withSudo(
					sudoStep("Remover "+hardeningFile, false, "rm", "-f", hardeningFile),
					sudoStep("Validar configuração (sshd -t)", false, "sshd", "-t"),
					sudoStep("Recarregar sshd", false, "systemctl", "try-reload-or-restart", "sshd.service"),
				)}
			}})
	} else {
		its = append(its, item{title: "Aplicar hardening", desc: "Só login por chave: desativa senha e login de root",
			job: func() job { return hardenJob() }})
	}
	return its
}

func hardenJob() job {
	const conf = "# Gerado pelo dotfiles CLI\nPasswordAuthentication no\nKbdInteractiveAuthentication no\nPermitRootLogin no\n"
	return job{title: "Aplicar hardening", steps: withSudo(
		nativeStep("Conferir ~/.ssh/authorized_keys", func() error {
			if authorizedKeys() == 0 {
				return errors.New("nenhuma chave em ~/.ssh/authorized_keys: adicione uma antes para não perder o acesso")
			}
			return nil
		}),
		step{label: "Gravar " + hardeningFile, run: func() (string, error) { return sudoWrite(hardeningFile, conf) }},
		step{label: "Validar configuração (sshd -t)", run: func() (string, error) {
			out, err := sudoRun("sshd", "-t")
			if err != nil {
				sudoRun("rm", "-f", hardeningFile)
				out += "\nConfiguração inválida: " + hardeningFile + " foi removido."
			}
			return out, err
		}},
		sudoStep("Recarregar sshd", false, "systemctl", "try-reload-or-restart", "sshd.service"),
	)}
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
			"⚠ O firewall está ativo e não libera a porta "+s.port()+". Use Firewall › Liberar SSH."))
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
	f := m.fw
	port := f.port
	var its []item

	if f.active() {
		its = append(its, item{title: "Desativar firewall", desc: "Desliga o UFW (as regras ficam salvas)",
			job: func() job {
				return job{title: "Desativar firewall", steps: withSudo(sudoStep("ufw disable", false, "ufw", "disable"))}
			}})
	} else {
		its = append(its, item{title: "Ativar firewall", desc: "Bloqueia entrada, libera saída e liga no boot",
			job: func() job { return enableFirewallJob(port) }})
	}

	its = append(its,
		item{title: "Liberar SSH", desc: fmt.Sprintf("Porta %s/tcp aberta para qualquer origem", port),
			job: func() job {
				return fwRuleJob("Liberar SSH", sudoStep("Liberar "+port+"/tcp", false, "ufw", "allow", port+"/tcp", "comment", "SSH"))
			}},
		item{title: "Liberar SSH só na LAN", desc: fmt.Sprintf("Porta %s/tcp só para %s", port, f.lan),
			job: func() job {
				return fwRuleJob("Liberar SSH só na LAN", sudoStep("Liberar "+port+"/tcp para "+f.lan, false,
					"ufw", "allow", "from", f.lan, "to", "any", "port", port, "proto", "tcp", "comment", "SSH LAN"))
			}},
		item{title: "Fechar SSH", desc: fmt.Sprintf("Remove as regras que liberam a porta %s", port),
			job: func() job { return closeSSHJob(f) }},
	)
	return its
}

func fwRuleJob(title string, rule step) job {
	steps := append(ensurePkgs("ufw", "iptables-nft"), rule, sudoStep("Recarregar UFW", true, "ufw", "reload"))
	return job{title: title, steps: withSudo(steps...)}
}

func enableFirewallJob(port string) job {
	steps := ensurePkgs("ufw", "iptables-nft")
	steps = append(steps,
		sudoStep("Entrada: bloquear por padrão", false, "ufw", "default", "deny", "incoming"),
		sudoStep("Saída: permitir por padrão", false, "ufw", "default", "allow", "outgoing"))
	// Numa sessão remota, liberar o SSH antes de ativar evita perder o acesso.
	if remoteSession() {
		steps = append(steps, sudoStep("Liberar "+port+"/tcp (sessão SSH atual)", false, "ufw", "allow", port+"/tcp", "comment", "SSH"))
	}
	steps = append(steps,
		sudoStep("Habilitar ufw.service no boot", false, "systemctl", "enable", "--now", "ufw.service"),
		sudoStep("Ativar UFW", false, "ufw", "--force", "enable"))
	return job{title: "Ativar firewall", steps: withSudo(steps...), result: func() string {
		fw := loadFWStatus(port)
		if loadSSHStatus().service.isActive() && fw.rulesErr == nil && !fw.sshAllowed() {
			return lipgloss.NewStyle().Foreground(colWarn).Render("⚠ O sshd está rodando mas a porta " + port + " está fechada. Use Liberar SSH.")
		}
		return ""
	}}
}

// closeSSHJob gera um `ufw delete` exato para cada regra que libera a porta.
func closeSSHJob(f fwStatus) job {
	var steps []step
	seen := map[string]bool{}
	for _, r := range f.v4Rules() {
		if !r.allowsPort(f.port) {
			continue
		}
		args := []string{"ufw", "delete", "allow"}
		spec := f.port
		if r.proto != "any" {
			spec += "/" + r.proto
		}
		if r.anySource() {
			args = append(args, spec)
		} else {
			args = append(args, "from", r.src, "to", "any", "port", f.port)
			if r.proto != "any" {
				args = append(args, "proto", r.proto)
			}
		}
		key := strings.Join(args, " ")
		if !seen[key] {
			seen[key] = true
			steps = append(steps, sudoStep("Remover "+describeRule(r), false, args...))
		}
	}
	if len(steps) == 0 {
		return job{title: "Fechar SSH", steps: []step{nativeStep("Nenhuma regra libera a porta "+f.port, func() error { return nil })}}
	}
	steps = append(steps, sudoStep("Recarregar UFW", true, "ufw", "reload"))
	return job{title: "Fechar SSH", steps: withSudo(steps...)}
}
