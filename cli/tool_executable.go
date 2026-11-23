package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

func findExecutable(lookup func(string) (string, error), name, mise string, fallback ...string) string {
	miseInfo, _ := os.Stat(mise)
	isMiseShim := func(path string) bool {
		if info, err := os.Stat(path); err == nil && os.SameFile(info, miseInfo) {
			return true
		}
		target, err := os.Readlink(path)
		return err == nil && filepath.Base(target) == "mise"
	}
	if path, err := lookup(name); err == nil && filepath.IsAbs(path) {
		if !isMiseShim(path) {
			return path
		}
		// A shim may hide an executable further down PATH.
		for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
			if !filepath.IsAbs(dir) {
				continue
			}
			candidate := filepath.Join(dir, name)
			if path, err := lookup(candidate); err == nil && filepath.IsAbs(path) && !isMiseShim(path) {
				return path
			}
		}
	}
	for _, candidate := range fallback {
		if path, err := lookup(candidate); err == nil && filepath.IsAbs(path) && !isMiseShim(path) {
			return path
		}
	}
	return ""
}

func findMise(home string, lookup func(string) (string, error)) string {
	for _, name := range []string{"mise", filepath.Join(home, ".local", "bin", "mise"), "/usr/bin/mise"} {
		if path, err := lookup(name); err == nil && filepath.IsAbs(path) {
			return path
		}
	}
	return ""
}

func miseLookup(commands sshCommands, prefix []string, name string, tool ...string) string {
	args := append(prefix, "--quiet", "which", name)
	if len(tool) != 0 {
		args = append(args, "--tool", tool[0])
	}
	if output, err := commands.capture("", args...); err == nil {
		path := strings.TrimSpace(output)
		if filepath.IsAbs(path) && !strings.ContainsAny(path, "\r\n") {
			return path
		}
	}
	return ""
}

// Use an installed version without activating a project's configuration or downloading tools.
func installedMiseTool(commands sshCommands, mise, name, tool string) []string {
	if mise == "" {
		return nil
	}
	prefix := []string{"env", "MISE_AUTO_INSTALL=0", mise, "--cd", "/"}
	if path := miseLookup(commands, prefix, name); path != "" {
		return append(prefix, "exec", "--", path)
	}
	version, err := commands.capture("", append(prefix, "--quiet", "latest", "--installed", tool)...)
	if fields := strings.Fields(version); err == nil && len(fields) == 1 {
		pinned := tool + "@" + fields[0]
		if path := miseLookup(commands, prefix, name, pinned); path != "" {
			return append(prefix, "exec", pinned, "--", path)
		}
	}
	return nil
}

func ensureMise(c *sshConsole, lookup func(string) (string, error), packages ...string) (string, error) {
	mise := findMise(c.home, lookup)
	if mise == "" {
		packages = append([]string{"mise"}, packages...)
	}
	if err := c.ensurePackages(packages...); err != nil {
		return "", err
	}
	if mise == "" {
		mise = findMise(c.home, lookup)
		if mise == "" {
			return "", errors.New("executável do Mise indisponível após a instalação")
		}
	}
	return mise, nil
}
