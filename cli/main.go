// Comando dotfiles: configuração interativa por perfil.
package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const version = "1.37.0"

type profile struct {
	prefix      string
	name        string
	environment string
}

func osReleaseID(data string) string {
	for line := range strings.SplitSeq(data, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && key == "ID" {
			return strings.Trim(value, "\"'")
		}
	}
	return ""
}

func systemOSID() string {
	data, _ := os.ReadFile("/etc/os-release")
	return osReleaseID(string(data))
}

func identifyProfile(hostname string) profile {
	const cachyos = "CachyOS · Niri · Noctalia Shell"
	profiles := map[string]profile{
		"desktop":  {"desktop", "Desktop", cachyos},
		"laptop":   {"laptop", "Laptop", cachyos},
		"homeprod": {"homeprod", "HomeProd", "Ubuntu"},
		"homelab":  {"homelab", "Homelab", "Ubuntu"},
	}
	var detected profile
	for part := range strings.SplitSeq(strings.ToLower(hostname), "-") {
		candidate, ok := profiles[part]
		if !ok {
			continue
		}
		if detected.prefix != "" && detected.prefix != candidate.prefix {
			return profile{}
		}
		detected = candidate
	}
	return detected
}

func (p profile) sshKeyName() string {
	switch p.prefix {
	case "desktop", "laptop":
		return "id_" + p.prefix
	default:
		return ""
	}
}

func main() {
	hostname, _ := os.Hostname()
	p := identifyProfile(hostname)
	if action := consoleActionForArgs(os.Args[1:]); action != nil {
		if err := action(p, terminalConsole()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if _, err := tea.NewProgram(newMenu(hostname), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}
