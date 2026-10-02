package main

import (
	"slices"
	"testing"
)

func TestTerminalToolsIncludeRequestedPackages(t *testing.T) {
	want := []string{"vim", "neovim", "wget", "curl", "bat", "eza", "tree"}
	if !slices.Equal(terminalToolPackages, want) {
		t.Fatalf("pacotes inesperados: %v", terminalToolPackages)
	}
}
