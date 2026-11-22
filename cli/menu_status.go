package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

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

type networkStatus struct {
	loaded, sshReady          bool
	sshInstalled, sshActive   bool
	sshEnabled, socketActive  bool
	homelab, socketEnabled    bool
	ufwInstalled, ufwEnabled  bool
	ufwActive                 bool
	port, password, root      string
	keys                      int
	address, lan              string
	inputPolicy, outputPolicy string
	rules                     []ufwRule
	rulesErr                  error
}

func readNetworkStatus(p profile) networkStatus {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	commands := sshCommandsWithContext(ctx)
	_, sshErr := exec.LookPath("sshd")
	_, ufwErr := exec.LookPath("ufw")
	s := networkStatus{
		loaded:       true,
		homelab:      p.prefix == "homelab",
		sshInstalled: sshErr == nil,
		ufwInstalled: ufwErr == nil,
		address:      "nenhuma interface de rede ativa",
		lan:          "não detectada",
	}
	if lan, err := detectLAN(); err == nil {
		s.lan = lan.subnet.String()
		s.address = s.lan + " (" + lan.device + ")"
		if device, err := net.InterfaceByName(lan.device); err == nil {
			if addrs, err := device.Addrs(); err == nil {
				for _, addr := range addrs {
					if ip, ok := addr.(*net.IPNet); ok && lan.subnet.Contains(ip.IP) {
						s.address = ip.IP.String() + " (" + lan.device + ")"
						break
					}
				}
			}
		}
	}
	if s.sshInstalled {
		service, socket := s.sshUnits()
		s.sshActive = isActive(service, commands)
		s.sshEnabled = isEnabled(service, commands)
		s.socketActive = isActive(socket, commands)
		s.socketEnabled = isEnabled(socket, commands)
		if data, err := os.ReadFile("/etc/ssh/sshd_config"); err == nil {
			s.port, _ = configuredSSHPort("/etc/ssh/sshd_config")
			var cfg = make(map[string]string)
			for _, line := range strings.Split(normalizedSSHConfig(string(data)), "\n") {
				key, val, _ := strings.Cut(line, " ")
				if key == "match" {
					break
				}
				if _, ok := cfg[key]; !ok {
					cfg[key] = val
				}
			}
			s.password, s.root = cfg["passwordauthentication"], cfg["permitrootlogin"]
			if account, err := user.Current(); err == nil {
				directory := filepath.Join(account.HomeDir, ".ssh")
				if authorized, err := readAuthorizedKeys(filepath.Join(directory, "authorized_keys"), commands); err == nil {
					s.keys = len(authorized.indices)
					s.sshReady = !s.homelab && s.sshActive && s.sshEnabled && !s.socketActive &&
						!s.socketEnabled && s.port != "" &&
						verifySSHPolicy(string(data), account.Username) == nil &&
						clientReady(directory, p.sshKeyName()) && probeSSHPort(s.port) == nil
				}
			}
		}
	}
	if s.ufwInstalled {
		config, _ := os.ReadFile("/etc/ufw/ufw.conf")
		defaults, _ := os.ReadFile("/etc/default/ufw")
		s.ufwEnabled = ufwSetting(config, "ENABLED") == "yes"
		s.ufwActive = isActive("ufw.service", commands)
		s.inputPolicy = ufwSetting(defaults, "DEFAULT_INPUT_POLICY")
		s.outputPolicy = ufwSetting(defaults, "DEFAULT_OUTPUT_POLICY")
		s.rules, s.rulesErr = readUFWRules()
	}
	return s
}

func (s networkStatus) sshUnits() (service, socket string) {
	if s.homelab {
		return "ssh.service", "ssh.socket"
	}
	return "sshd.service", "sshd.socket"
}

func clientReady(directory, keyName string) bool {
	for _, name := range []string{keyName, keyName + ".pub", "config", "authorized_keys"} {
		if err := sshRegular(filepath.Join(directory, name), false); err != nil {
			return false
		}
	}
	config, err := os.ReadFile(filepath.Join(directory, "config"))
	return err == nil && string(config) == clientSSHConfig(keyName)
}

func (s networkStatus) card(page screen, width int, compact bool) string {
	if !s.loaded {
		return renderCard([]cardRow{{colDim, "Estado", "carregando…"}}, width)
	}
	if page == screenFirewall {
		return renderCard(s.firewallRows(compact), width)
	}
	return renderCard(s.sshRows(), width)
}

func (s networkStatus) sshRows() []cardRow {
	var rows []cardRow
	service, socket := s.sshUnits()
	if !s.sshInstalled {
		name := "openssh"
		if s.homelab {
			name = "openssh-server"
		}
		rows = append(rows, cardRow{colErr, "Pacote", name + " não instalado"})
	}
	switch {
	case s.sshActive:
		rows = append(rows, cardRow{colOK, "Servidor", "rodando (" + service + ")"})
	case s.socketActive:
		rows = append(rows, cardRow{colWarn, "Servidor", "escutando via " + socket})
	default:
		rows = append(rows, cardRow{colDim, "Servidor", "parado"})
	}
	if s.sshEnabled {
		rows = append(rows, cardRow{colOK, "Boot", service + " habilitado"})
	} else if s.homelab && s.socketEnabled {
		rows = append(rows, cardRow{colOK, "Boot", socket + " habilitado"})
	} else {
		rows = append(rows, cardRow{colDim, "Boot", "não inicia no boot"})
	}
	if s.port == "" || s.password == "" || s.root == "" {
		rows = append(rows, cardRow{colWarn, "Login", "política SSH não verificada"})
	} else {
		color, password := colWarn, "sim"
		if s.password == "no" {
			color, password = colOK, "não"
		}
		rows = append(rows, cardRow{color, "Login", fmt.Sprintf("porta %s · senha: %s · root: %s", s.port, password, s.root)})
	}
	rows = append(rows,
		cardRow{colOK, "Chaves", fmt.Sprintf("%d em ~/.ssh/authorized_keys", s.keys)},
		cardRow{colDim, "Endereço", s.address})
	return rows
}

func (s networkStatus) firewallRows(compact bool) []cardRow {
	lan := cardRow{colDim, "LAN", s.lan}
	if !s.ufwInstalled {
		return []cardRow{{colErr, "Pacote", "ufw não instalado"}, lan}
	}
	var rows []cardRow
	switch {
	case s.ufwEnabled && s.ufwActive:
		rows = append(rows, cardRow{colOK, "Firewall", "ativo"})
	case s.ufwEnabled:
		rows = append(rows, cardRow{colWarn, "Firewall", "ativado, mas ufw.service está parado"})
	default:
		rows = append(rows, cardRow{colWarn, "Firewall", "inativo"})
	}
	if s.inputPolicy != "" && s.outputPolicy != "" {
		rows = append(rows, cardRow{colDim, "Política", fmt.Sprintf("entrada %s · saída %s", strings.ToLower(s.inputPolicy), strings.ToLower(s.outputPolicy))})
	}
	if s.rulesErr != nil {
		return append(rows, cardRow{colWarn, "Regras", "sem permissão para ler as regras salvas do UFW"}, lan)
	}
	if s.port != "" {
		if slices.ContainsFunc(s.rules, func(rule ufwRule) bool { return rule.dport == s.port && rule.comment == ufwSSHAllow }) {
			rows = append(rows, cardRow{colOK, "SSH", "porta " + s.port + " liberada na LAN"})
		} else {
			rows = append(rows, cardRow{colWarn, "SSH", "regra da LAN pendente para a porta " + s.port})
		}
	}
	if compact {
		return append(rows, lan)
	}
	if len(s.rules) == 0 {
		rows = append(rows, cardRow{colDim, "Regras", "nenhuma"})
	}
	for i, rule := range s.rules {
		label := ""
		if i == 0 {
			label = "Regras"
		}
		if i == 4 && len(s.rules) > 5 {
			rows = append(rows, cardRow{"", label, fmt.Sprintf("… e mais %d", len(s.rules)-4)})
			break
		}
		rows = append(rows, cardRow{colDim, label, rule.description()})
	}
	return append(rows, lan)
}

type ufwRule struct {
	action, proto, dport, src, comment string
}

func (r ufwRule) description() string {
	src := r.src
	if src == "0.0.0.0/0" || src == "::/0" {
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

func readUFWRules() ([]ufwRule, error) {
	var rules []ufwRule
	for _, file := range []string{"/etc/ufw/user.rules", "/etc/ufw/user6.rules"} {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(b), "\n") {
			rest, ok := strings.CutPrefix(line, "### tuple ### ")
			if !ok {
				continue
			}
			f := strings.Fields(rest)
			if len(f) < 7 || (strings.HasSuffix(file, "6.rules") && f[5] == "::/0" && f[3] == "::/0") {
				continue
			}
			r := ufwRule{action: f[0], proto: f[1], dport: f[2], src: f[5]}
			dirIn := false
			for _, tok := range f[6:] {
				if c, ok := strings.CutPrefix(tok, "comment="); ok {
					if dec, err := hex.DecodeString(c); err == nil {
						r.comment = string(dec)
					}
				}
				if tok == "in" || strings.HasPrefix(tok, "in_") {
					dirIn = true
				}
			}
			if dirIn {
				rules = append(rules, r)
			}
		}
	}
	return rules, nil
}
