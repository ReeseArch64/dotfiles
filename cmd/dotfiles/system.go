package main

// Leitura nativa do estado do sistema: nada aqui precisa de root.

import (
	"bufio"
	"context"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	sdbus "github.com/coreos/go-systemd/v22/dbus"
)

// ---- pacman ----

// missingPkgs devolve os pacotes que não estão instalados nem são providos por
// outro pacote (ex.: iptables provê iptables-nft), como `pacman -T`.
func missingPkgs(names ...string) []string {
	have := map[string]bool{}
	entries, _ := os.ReadDir("/var/lib/pacman/local")
	for _, e := range entries {
		f, err := os.Open(filepath.Join("/var/lib/pacman/local", e.Name(), "desc"))
		if err != nil {
			continue
		}
		section := ""
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "%"):
				section = line
			case line != "" && (section == "%NAME%" || section == "%PROVIDES%"):
				name, _, _ := strings.Cut(line, "=")
				have[name] = true
			}
		}
		f.Close()
	}
	var missing []string
	for _, n := range names {
		if !have[n] {
			missing = append(missing, n)
		}
	}
	return missing
}

// ---- systemd (D-Bus) ----

type unitState struct{ active, enabled string }

func (u unitState) isActive() bool  { return u.active == "active" }
func (u unitState) isEnabled() bool { return u.enabled == "enabled" }

func unitStates(names ...string) map[string]unitState {
	out := map[string]unitState{}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	conn, err := sdbus.NewSystemConnectionContext(ctx)
	if err != nil {
		return out
	}
	defer conn.Close()
	for _, n := range names {
		p, err := conn.GetUnitPropertiesContext(ctx, n)
		if err != nil {
			continue
		}
		a, _ := p["ActiveState"].(string)
		e, _ := p["UnitFileState"].(string)
		out[n] = unitState{a, e}
	}
	return out
}

// ---- sshd_config ----

const hardeningFile = "/etc/ssh/sshd_config.d/10-hardening.conf"

// readSSHDConfig resolve as diretivas globais como o sshd faz: primeira
// ocorrência vence, Include é expandido no lugar e blocos Match são ignorados.
// partial indica que algum arquivo não pôde ser lido.
func readSSHDConfig() (cfg map[string]string, partial bool) {
	cfg = map[string]string{}
	var parse func(path string) bool
	parse = func(path string) bool {
		f, err := os.Open(path)
		if err != nil {
			partial = true
			return true
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, val, _ := strings.Cut(strings.Replace(line, "=", " ", 1), " ")
			key, val = strings.ToLower(key), strings.TrimSpace(val)
			switch key {
			case "match":
				return false
			case "include":
				for _, pat := range strings.Fields(val) {
					if !filepath.IsAbs(pat) {
						pat = filepath.Join("/etc/ssh", pat)
					}
					matches, _ := filepath.Glob(pat)
					slices.Sort(matches)
					for _, m := range matches {
						if !parse(m) {
							return false
						}
					}
				}
			default:
				if _, ok := cfg[key]; !ok {
					cfg[key] = strings.Trim(val, `"`)
				}
			}
		}
		return true
	}
	parse("/etc/ssh/sshd_config")

	defaults := map[string]string{"port": "22", "passwordauthentication": "yes", "permitrootlogin": "prohibit-password"}
	for k, v := range defaults {
		if _, ok := cfg[k]; !ok {
			cfg[k] = v
		}
	}
	return cfg, partial
}

func authorizedKeys() int {
	home, _ := os.UserHomeDir()
	b, err := os.ReadFile(filepath.Join(home, ".ssh", "authorized_keys"))
	if err != nil {
		return 0
	}
	n := 0
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasPrefix(l, "#") {
			n++
		}
	}
	return n
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// ---- rede ----

type ifaceAddr struct {
	name string
	net  *net.IPNet
}

var virtualIfPrefixes = []string{"lo", "docker", "br-", "veth", "virbr", "tailscale", "tun", "wg", "vnet"}

// lanAddrs lista os IPv4 das interfaces físicas que estão de pé.
func lanAddrs() []ifaceAddr {
	var out []ifaceAddr
	ifs, _ := net.Interfaces()
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || slices.ContainsFunc(virtualIfPrefixes, func(p string) bool { return strings.HasPrefix(i.Name, p) }) {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
				out = append(out, ifaceAddr{i.Name, ipn})
			}
		}
	}
	return out
}

// lanSubnet devolve a rede (ex.: 192.168.1.0/24) da primeira interface com IP privado.
func lanSubnet() string {
	for _, a := range lanAddrs() {
		if a.net.IP.IsPrivate() {
			return (&net.IPNet{IP: a.net.IP.Mask(a.net.Mask), Mask: a.net.Mask}).String()
		}
	}
	return "192.168.1.0/24"
}

func remoteSession() bool { return os.Getenv("SSH_CONNECTION") != "" }

// ---- UFW ----

type ufwRule struct {
	action, proto, dport, src, comment string
	v6                                 bool
}

func (r ufwRule) anySource() bool { return r.src == "0.0.0.0/0" || r.src == "::/0" }

func (r ufwRule) allowsPort(port string) bool {
	return strings.HasPrefix(r.action, "allow") && (r.proto == "tcp" || r.proto == "any") &&
		slices.Contains(strings.Split(r.dport, ","), port)
}

// readUFWRules lê as regras de entrada salvas pelo ufw (formato "### tuple ###").
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
			if len(f) < 7 {
				continue
			}
			r := ufwRule{action: f[0], proto: f[1], dport: f[2], src: f[5], v6: strings.HasSuffix(file, "6.rules")}
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

func readKV(path, key string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), key+"="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return ""
}
