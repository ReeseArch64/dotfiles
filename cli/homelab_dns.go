package main

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

const resolvedService = "systemd-resolved.service"

type homelabDNS struct {
	config, resolver, stub, upstream string
	dropIns                          []string
}

func homelabDNSPaths() homelabDNS {
	return homelabDNS{
		config: "/etc/systemd/resolved.conf", resolver: "/etc/resolv.conf",
		stub: "/run/systemd/resolve/stub-resolv.conf", upstream: "/run/systemd/resolve/resolv.conf",
		dropIns: []string{"/etc/systemd/resolved.conf.d", "/run/systemd/resolved.conf.d", "/usr/local/lib/systemd/resolved.conf.d", "/usr/lib/systemd/resolved.conf.d"},
	}
}

func dnsStubSetting(line string) (value string, ok bool) {
	key, value, ok := strings.Cut(line, "=")
	return strings.TrimSpace(value), ok && strings.TrimSpace(key) == "DNSStubListener"
}

func checkDNSDropIns(directories []string) error {
	for _, directory := range directories {
		files, err := os.ReadDir(directory)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".conf") {
				continue
			}
			path := filepath.Join(directory, file.Name())
			if target, err := filepath.EvalSymlinks(path); err == nil && target == "/dev/null" {
				continue
			}
			info, err := os.Stat(path)
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("drop-in DNS precisa ser um arquivo regular: %s", path)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			resolve := false
			for line := range strings.SplitSeq(string(data), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "[") {
					resolve = line == "[Resolve]"
				}
				if value, ok := dnsStubSetting(line); resolve && ok {
					switch strings.ToLower(value) {
					case "no", "false", "off", "0":
					default:
						return fmt.Errorf("DNSStubListener conflitante em %s", path)
					}
				}
			}
		}
	}
	return nil
}

func disableDNSStub(data []byte) []byte {
	var out strings.Builder
	resolve, added, continuation := false, false, false
	for raw := range strings.SplitAfterSeq(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if continuation {
			continuation = strings.HasSuffix(line, "\\")
			continue
		}
		if strings.HasPrefix(line, "[") {
			resolve = line == "[Resolve]"
			if resolve && !added {
				out.WriteString(strings.TrimRight(raw, "\r\n"))
				out.WriteString("\nDNSStubListener=no\n")
				added = true
				continue
			}
		}
		if _, ok := dnsStubSetting(line); resolve && ok {
			continuation = strings.HasSuffix(line, "\\")
			continue
		}
		out.WriteString(raw)
	}
	if !added {
		if out.Len() != 0 && !strings.HasSuffix(out.String(), "\n") {
			out.WriteByte('\n')
		}
		out.WriteString("\n[Resolve]\nDNSStubListener=no\n")
	}
	return []byte(out.String())
}

func verifyResolver(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	found := false
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "nameserver" {
			continue
		}
		host, _, _ := strings.Cut(fields[1], "%")
		if ip := net.ParseIP(host); ip != nil {
			if ip.Equal(net.IPv4(127, 0, 0, 53)) || ip.Equal(net.IPv4(127, 0, 0, 54)) {
				return fmt.Errorf("%s ainda depende do stub DNS local", path)
			}
			found = true
		}
	}
	if !found {
		return fmt.Errorf("nenhum servidor DNS válido em %s", path)
	}
	return nil
}

func (d homelabDNS) resolverLink() (string, error) {
	info, err := os.Lstat(d.resolver)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("resolv.conf precisa ser um arquivo regular ou link: %s", d.resolver)
		}
		return "", nil
	}
	link, err := os.Readlink(d.resolver)
	if err != nil {
		return "", err
	}
	target := link
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(d.resolver), target)
	}
	resolved, _ := filepath.EvalSymlinks(d.resolver)
	if filepath.Clean(target) == d.stub || resolved == d.stub {
		return link, nil
	}
	return "", nil
}

func replaceDNSLink(path, from, to string) error {
	current, err := os.Readlink(path)
	if err != nil || current != from {
		return fmt.Errorf("o link DNS mudou durante a operação: %s", path)
	}
	staging, err := os.MkdirTemp(filepath.Dir(path), ".dotfiles-dns-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	newLink := filepath.Join(staging, "resolv.conf")
	if err := os.Symlink(to, newLink); err != nil {
		return err
	}
	return os.Rename(newLink, path)
}

func (d homelabDNS) apply(commands, recovery sshCommands, save func(homelabBackup) (string, error)) (result error) {
	if err := checkDNSDropIns(d.dropIns); err != nil {
		return err
	}
	original, mode, err := readHomelabSystemFile(d.config)
	if err != nil {
		return err
	}
	link, err := d.resolverLink()
	if err != nil {
		return err
	}
	check := d.resolver
	if link != "" {
		check = d.upstream
	}
	if err := verifyResolver(check); err != nil {
		return err
	}
	updated := disableDNSStub(original)
	changed := !bytes.Equal(original, updated)
	if changed || link != "" {
		backup, err := save(homelabBackup{Contents: original, ResolverLink: link})
		if err != nil {
			return err
		}
		fmt.Println("Backup do DNS:", backup)
	}
	wasActive := isActive(resolvedService, commands)
	switched, written, restarted := false, false, false
	defer func() {
		if result == nil {
			return
		}
		if switched {
			result = errors.Join(result, replaceDNSLink(d.resolver, d.upstream, link))
		}
		if written {
			result = errors.Join(result, writeConfigAtomic(d.config, original, mode))
		}
		if restarted {
			action := "stop"
			if wasActive {
				action = "restart"
			}
			_, err := recovery.capture("", "systemctl", action, resolvedService)
			result = errors.Join(result, err)
		}
	}()
	if link != "" {
		if err := replaceDNSLink(d.resolver, link, d.upstream); err != nil {
			return err
		}
		switched = true
	}
	if changed {
		if err := writeConfigAtomic(d.config, updated, mode); err != nil {
			return err
		}
		written = true
	}
	restarted = true
	if _, err := commands.capture("", "systemctl", "restart", resolvedService); err != nil {
		return err
	}
	if err := verifyResolver(d.resolver); err != nil {
		return err
	}
	fmt.Println("Stub DNS do systemd-resolved desativado. O resolv.conf aponta para servidores DNS válidos.")
	return nil
}
