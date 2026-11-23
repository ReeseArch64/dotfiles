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
)

func (d deviceProfile) String() string {
	switch d {
	case deviceDesktop:
		return "Desktop"
	case deviceNotebook:
		return "Notebook"
	case deviceMacBook:
		return "MacBook"
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
		profile = "notebook"
	case deviceMacBook:
		profile = "macbook"
	default:
		return ""
	}
	return filepath.Join(dotfiles, "setup", profile)
}

func detectDevice(hostname string) (deviceProfile, error) {
	normalized := strings.ToLower(hostname)
	matches := make([]deviceProfile, 0, 3)
	for marker, profile := range map[string]deviceProfile{
		"desktop":  deviceDesktop,
		"notebook": deviceNotebook,
		"macbook":  deviceMacBook,
	} {
		if strings.Contains(normalized, marker) {
			matches = append(matches, profile)
		}
	}
	if len(matches) != 1 {
		return deviceUnknown, fmt.Errorf("hostname %q deve conter exatamente um destes identificadores: desktop, notebook ou macbook; nenhuma opção disponível", hostname)
	}
	return matches[0], nil
}
