// Comando dotfiles: configuração interativa por perfil.
package main

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const version = "1.35.0"

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
	if len(os.Args) > 1 && os.Args[1] == "--homelab" {
		if err := runHomelabAction(identifyProfile(hostname), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--network" {
		if err := runNetworkAction(identifyProfile(hostname), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "--packages" {
		if err := runPackageAction(identifyProfile(hostname), os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "--terminal-config" {
		if err := runTerminalConfig(identifyProfile(hostname), os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 3 && os.Args[1] == "--mise-login" {
		if err := runMiseLogin(identifyProfile(hostname), os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) == 2 {
		var action func(profile) error
		switch os.Args[1] {
		case "--cursor":
			action = runCursorAction
		case "--wallpapers":
			action = runWallpapersAction
		case "--setup-noctalia":
			action = runNoctaliaSetup
		case "--sdk-links":
			action = runSDKMigration
		case "--storage-links":
			action = runStorageMigration
		case "--setup-workspaces":
			action = runWorkspacesSetup
		case "--setup-niri":
			action = runNiriSetup
		case "--setup-git":
			action = runGitSetup
		case "--setup-mise":
			action = runMiseSetup
		case "--setup-pi-agent":
			action = runPiAgentSetup
		case "--remove-preinstalled":
			action = runPreinstalledCleanup
		case "--setup-docker":
			action = runDockerSetup
		case "--setup-distrobox":
			action = runDistroboxSetup
		case "--apply-docker":
			action = runPrivilegedDocker
		case "--docker-backup":
			action = writeDockerBackup
		case "--sshd-backup":
			action = writeSSHDBackup
		}
		if action != nil {
			if err := action(identifyProfile(hostname)); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			return
		}
	}
	if _, err := tea.NewProgram(newMenu(hostname), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}
