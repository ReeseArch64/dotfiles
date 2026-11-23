package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
)

type localNetwork struct {
	device string
	subnet *net.IPNet
}

func (lan localNetwork) checkConnection(connection string) error {
	if connection == "" {
		return nil
	}
	fields := strings.Fields(connection)
	if len(fields) != 4 || !lan.subnet.Contains(net.ParseIP(fields[0])) || !lan.subnet.Contains(net.ParseIP(fields[2])) {
		return errors.New("a sessão SSH atual está fora da LAN detectada; aplique o firewall pelo console local")
	}
	return nil
}

func defaultRouteInterface(content string) string {
	best, bestMetric := "", int(^uint(0)>>1)
	for _, line := range strings.Split(content, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 7 || fields[1] != "00000000" {
			continue
		}
		flags, flagErr := strconv.ParseUint(fields[3], 16, 32)
		metric, metricErr := strconv.Atoi(fields[6])
		if flagErr != nil || metricErr != nil || flags&1 == 0 || metric >= bestMetric {
			continue
		}
		best, bestMetric = fields[0], metric
	}
	return best
}

func detectLAN() (localNetwork, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return localNetwork{}, err
	}
	routes, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return localNetwork{}, err
	}
	preferred := defaultRouteInterface(string(routes))
	virtual := []string{"docker", "br-", "veth", "virbr", "podman", "cni", "incus", "lxc", "waydroid", "tailscale", "tun", "wg", "vnet", "zt"}
	var candidates []localNetwork
	for _, device := range interfaces {
		if device.Flags&net.FlagUp == 0 || device.Flags&net.FlagLoopback != 0 ||
			slices.ContainsFunc(virtual, func(prefix string) bool { return strings.HasPrefix(device.Name, prefix) }) {
			continue
		}
		addresses, err := device.Addrs()
		if err != nil {
			return localNetwork{}, err
		}
		for _, addr := range addresses {
			subnet, ok := addr.(*net.IPNet)
			if !ok || !privateIPv4Subnet(subnet) {
				continue
			}
			candidate := localNetwork{device.Name, &net.IPNet{IP: subnet.IP.Mask(subnet.Mask), Mask: subnet.Mask}}
			if !slices.ContainsFunc(candidates, func(existing localNetwork) bool {
				return existing.device == candidate.device && existing.subnet.String() == candidate.subnet.String()
			}) {
				candidates = append(candidates, candidate)
			}
		}
	}
	return selectLAN(candidates, preferred)
}

func selectLAN(candidates []localNetwork, preferred string) (localNetwork, error) {
	var matches []localNetwork
	for _, candidate := range candidates {
		if candidate.device == preferred {
			matches = append(matches, candidate)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	return localNetwork{}, errors.New("não foi possível identificar uma LAN IPv4 privada inequívoca")
}

func privateIPv4Subnet(subnet *net.IPNet) bool {
	ip := subnet.IP.To4()
	if ip == nil || len(subnet.Mask) != net.IPv4len || !ip.IsPrivate() {
		return false
	}
	first := ip.Mask(subnet.Mask)
	last := append(net.IP(nil), first...)
	for i := range last {
		last[i] |= ^subnet.Mask[i]
	}
	return first.IsPrivate() && last.IsPrivate()
}

func isEnabled(unit string, commands sshCommands) bool {
	_, err := commands.capture("", "systemctl", "is-enabled", "--quiet", unit)
	return err == nil
}

func isActive(unit string, commands sshCommands) bool {
	_, err := commands.capture("", "systemctl", "is-active", "--quiet", unit)
	return err == nil
}

func firewallSSHPort(account, home, keyName string, commands sshCommands) (string, error) {
	directory := filepath.Join(home, ".ssh")
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return "", errors.New("configure primeiro o SSH: diretório ~/.ssh inválido ou com permissões inseguras")
	}
	for _, name := range []string{keyName, keyName + ".pub", "config", "authorized_keys"} {
		path := filepath.Join(directory, name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || (name != "authorized_keys" && info.Size() == 0) {
			return "", fmt.Errorf("configure primeiro o SSH: %s ausente ou inválido", name)
		}
		if (name == keyName || name == "authorized_keys") && info.Mode().Perm()&0077 != 0 {
			return "", fmt.Errorf("permissões SSH inseguras: %s", name)
		}
	}
	public, err := os.ReadFile(filepath.Join(directory, keyName+".pub"))
	if err != nil {
		return "", err
	}
	if _, _, err := validatePublicKey(strings.TrimSpace(string(public)), commands); err != nil {
		return "", err
	}
	if _, err := readAuthorizedKeys(filepath.Join(directory, "authorized_keys"), commands); err != nil {
		return "", err
	}
	config, err := os.ReadFile(filepath.Join(directory, "config"))
	if err != nil || string(config) != clientSSHConfig(keyName) {
		return "", errors.New("configure primeiro o cliente SSH pelo menu")
	}
	if !isActive("sshd.service", commands) || !isEnabled("sshd.service", commands) ||
		isActive("sshd.socket", commands) || isEnabled("sshd.socket", commands) {
		return "", errors.New("configure primeiro o SSH: sshd.service deve estar ativo e habilitado sem sshd.socket")
	}
	if _, err := commands.capture("", "sshd", "-t"); err != nil {
		return "", err
	}
	effective, err := dumpSSHConfig(commands, []string{"sshd"}, "-C", "user="+account+",host=localhost,addr=127.0.0.1")
	if err != nil {
		return "", err
	}
	if err := verifySSHPolicy(effective, account); err != nil {
		return "", err
	}
	port, err := effectiveSSHPort(effective)
	if err != nil {
		return "", err
	}
	return port, probeSSHPort(port)
}

const (
	ufwSSHAllow = "Dotfiles-SSH-LAN"
	ufwSSHDeny  = "Dotfiles-SSH-Outside-LAN"
)

type firewall struct {
	configDir, defaults string
	commands, recovery  sshCommands
}

type ufwFile struct {
	path string
	data []byte
	mode os.FileMode
}

func (f firewall) snapshot() ([]ufwFile, error) {
	paths := []string{f.defaults}
	for _, name := range []string{"ufw.conf", "user.rules", "user6.rules"} {
		paths = append(paths, filepath.Join(f.configDir, name))
	}
	var files []ufwFile
	for _, path := range paths {
		if err := sshRegular(path, false); err != nil {
			return nil, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		files = append(files, ufwFile{path, data, info.Mode().Perm()})
	}
	return files, nil
}

func ufwSetting(contents []byte, name string) string {
	for _, line := range strings.Split(string(contents), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && key == name {
			return strings.Trim(value, " '\"")
		}
	}
	return ""
}

func ufwManagedRules(output string) (rules [][]string, labels []string) {
	for _, line := range strings.Split(output, "\n") {
		line, ok := strings.CutPrefix(line, "ufw ")
		if !ok {
			continue
		}
		rule, val, _ := strings.Cut(line, " comment ")
		comment := strings.TrimSpace(strings.Trim(strings.TrimSpace(val), "'\""))
		if comment == ufwSSHAllow || comment == ufwSSHDeny {
			rules = append(rules, append(strings.Fields(rule), "comment", comment))
			labels = append(labels, comment)
		} else {
			labels = append(labels, "")
		}
	}
	return rules, labels
}

func (f firewall) apply(lan localNetwork, port string) error {
	files, err := f.snapshot()
	if err != nil {
		return err
	}
	if ufwSetting(files[0].data, "IPV6") != "yes" {
		return errors.New("habilite IPV6=yes em /etc/default/ufw para restringir também o SSH por IPv6")
	}
	run := func(args ...string) (string, error) { return f.commands.capture("", args...) }
	previous, err := run("ufw", "show", "added")
	if err != nil {
		return err
	}
	rules, _ := ufwManagedRules(previous)
	return f.applyWithRollback(files, func() error {
		for _, command := range [][]string{
			{"ufw", "default", "deny", "incoming"},
			{"ufw", "default", "allow", "outgoing"},
		} {
			if _, err := run(command...); err != nil {
				return err
			}
		}
		for _, rule := range rules {
			if _, err := run(append([]string{"ufw", "--force", "delete"}, rule...)...); err != nil {
				return err
			}
		}
		for _, command := range [][]string{
			{"ufw", "--force", "delete", "allow", port + "/tcp"},
			{"ufw", "prepend", "deny", port + "/tcp", "comment", ufwSSHDeny},
			{"ufw", "prepend", "allow", "in", "on", lan.device, "proto", "tcp", "from", lan.subnet.String(), "to", "any", "port", port, "comment", ufwSSHAllow},
		} {
			if _, err := run(command...); err != nil {
				return err
			}
		}
		added, err := run("ufw", "show", "added")
		if err != nil {
			return err
		}
		_, labels := ufwManagedRules(added)
		if len(labels) < 2 || labels[0] != ufwSSHAllow || labels[1] != ufwSSHDeny {
			return errors.New("não foi possível colocar as regras SSH antes das regras existentes no UFW")
		}
		for _, command := range [][]string{
			{"ufw", "--force", "enable"},
			{"systemctl", "enable", "--now", "ufw.service"},
			{"systemctl", "is-active", "--quiet", "ufw.service"},
		} {
			if _, err := run(command...); err != nil {
				return err
			}
		}
		return nil
	})
}

func (f firewall) applyWithRollback(files []ufwFile, apply func() error) (result error) {
	wasEnabled := isEnabled("ufw.service", f.commands)
	wasActive := isActive("ufw.service", f.commands)
	ufwWasOn := ufwSetting(files[1].data, "ENABLED") == "yes"
	defer func() {
		if result == nil {
			return
		}
		commands := f.recovery
		if commands.capture == nil {
			commands = f.commands
		}
		var rollbackErr error
		if !ufwWasOn {
			_, err := commands.capture("", "ufw", "--force", "disable")
			rollbackErr = errors.Join(rollbackErr, err)
		}
		// Stop a newly started unit before restoring the original state files.
		if !wasActive {
			_, err := commands.capture("", "systemctl", "stop", "ufw.service")
			rollbackErr = errors.Join(rollbackErr, err)
		}
		for _, file := range files {
			rollbackErr = errors.Join(rollbackErr, writeConfigAtomic(file.path, file.data, file.mode))
		}
		if ufwWasOn {
			_, err := commands.capture("", "ufw", "reload")
			rollbackErr = errors.Join(rollbackErr, err)
		}
		if !wasEnabled {
			_, err := commands.capture("", "systemctl", "disable", "ufw.service")
			rollbackErr = errors.Join(rollbackErr, err)
		}
		if rollbackErr != nil {
			result = errors.Join(result, fmt.Errorf("erro ao restaurar o UFW: %w", rollbackErr))
		}
	}()
	return apply()
}

func applyFirewall(connection string) error {
	account, err := sudoAccount()
	if err != nil {
		return err
	}
	lan, err := detectLAN()
	if err != nil {
		return err
	}
	if err := lan.checkConnection(connection); err != nil {
		return err
	}
	unlock, err := sshLock("system-ssh")
	if err != nil {
		return err
	}
	defer unlock()

	hostname, _ := os.Hostname()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	f := firewall{"/etc/ufw", "/etc/default/ufw", sshCommandsWithContext(ctx), systemSSHCommands()}
	port, err := firewallSSHPort(account.Username, account.HomeDir, identifyProfile(hostname).sshKeyName(), f.commands)
	if err != nil {
		return err
	}
	if err := f.apply(lan, port); err != nil {
		return err
	}
	fmt.Printf("UFW ativo. SSH na porta %s permitido pela LAN %s (%s).\n", port, lan.subnet, lan.device)
	return nil
}
