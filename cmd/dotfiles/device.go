package main

import (
	"fmt"
	"path/filepath"
	"strings"
)

type deviceProfile int

const (
	deviceUnknown deviceProfile = iota
	deviceDesktop
	deviceNotebook
	deviceMacBook
	deviceServer
	deviceHomeProd
)

func (d deviceProfile) String() string {
	switch d {
	case deviceDesktop:
		return "Desktop"
	case deviceNotebook:
		return "Notebook"
	case deviceMacBook:
		return "MacBook"
	case deviceServer:
		return "Homelab"
	case deviceHomeProd:
		return "HomeProd"
	default:
		return "Desconhecido"
	}
}

func setupDirectory(dotfiles string, device deviceProfile) string {
	var profile string
	switch device {
	case deviceDesktop:
		profile = "desktop"
	case deviceNotebook:
		profile = "laptop"
	case deviceMacBook:
		profile = "macbook"
	case deviceServer:
		profile = "homelab"
	case deviceHomeProd:
		profile = "homeprod"
	default:
		return ""
	}
	return filepath.Join(dotfiles, "setup", profile)
}

func detectDevice(hostname string) (deviceProfile, error) {
	normalized := strings.ToLower(hostname)
	matches := make([]deviceProfile, 0, 5)
	for marker, profile := range map[string]deviceProfile{
		"desktop":  deviceDesktop,
		"notebook": deviceNotebook,
		"macbook":  deviceMacBook,
		"homelab":  deviceServer,
		"homeprod": deviceHomeProd,
	} {
		if strings.Contains(normalized, marker) {
			matches = append(matches, profile)
		}
	}
	if len(matches) != 1 {
		return deviceUnknown, fmt.Errorf("hostname %q deve conter exatamente um destes identificadores: desktop, notebook, macbook, homelab ou homeprod; nenhuma opção disponível", hostname)
	}
	return matches[0], nil
}
