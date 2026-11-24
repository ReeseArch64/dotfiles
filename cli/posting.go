package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func (installer packageInstaller) installPosting() error {
	console := installer.console
	var candidates []string
	for _, variable := range []string{"UV_TOOL_BIN_DIR", "XDG_BIN_HOME"} {
		if directory := os.Getenv(variable); filepath.IsAbs(directory) {
			candidates = append(candidates, filepath.Join(directory, "posting"))
		}
	}
	if installer.userToolInstalled("posting", candidates...) {
		return nil
	}

	mise := findMise(console.home, installer.which)
	command := installedMiseTool(console.commands, mise, "uv", "uv")
	findUV := func() string {
		return findExecutable(installer.which, "uv", mise,
			filepath.Join(console.home, ".local", "bin", "uv"), "/usr/bin/uv")
	}
	if command == nil {
		if uv := findUV(); uv != "" {
			command = []string{uv}
		}
	}
	if command != nil {
		fmt.Fprintln(console.output, "Reutilizar uv em", command[len(command)-1])
	} else {
		fmt.Fprintln(console.output, "Pacotes dos repositórios: uv")
	}
	if command == nil {
		if err := console.ensurePackages("uv"); err != nil {
			return err
		}
		uv := findUV()
		if uv == "" {
			return errors.New("executável do uv indisponível após a instalação")
		}
		command = []string{uv}
	}
	return console.commands.interactive(append(command, "tool", "install", "--python", "3.13", "posting<3")...)
}
