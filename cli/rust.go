package main

import (
	"fmt"
	"strings"
)

func configureRust(console *sshConsole) error {
	defaultToolchain, err := console.commands.capture("", "rustup", "default")
	if err == nil && strings.HasPrefix(strings.TrimSpace(defaultToolchain), "stable-") {
		if err := verifyStableRust(console); err == nil {
			return nil
		}
	}
	if err := console.commands.interactive("rustup", "default", "stable"); err != nil {
		return fmt.Errorf("selecionar a toolchain Rust estável: %w", err)
	}
	return verifyStableRust(console)
}

func verifyStableRust(console *sshConsole) error {
	var versions []string
	for _, tool := range []string{"rustc", "cargo"} {
		output, err := console.commands.capture("", "rustup", "run", "stable", tool, "--version")
		if err != nil {
			return fmt.Errorf("verificar %s da toolchain estável: %w", tool, err)
		}
		versions = append(versions, strings.TrimSpace(output))
	}
	fmt.Fprintln(console.output, strings.Join(versions, "\n"))
	return nil
}
