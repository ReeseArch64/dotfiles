package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var developmentPacmanPackages = []string{"fish", "rustup", "tk"}

const betterStackInstallCommand = "curl -fsSL https://raw.githubusercontent.com/sounak98/betterstack-cli/main/install.sh | sh"

func developmentToolsInstallStep(dotfiles string) step {
	return terminalStep(
		"Instalar ferramentas configuradas via mise",
		"mise", "--cd", dotfiles, "install",
	)
}

func miseToolOutput(home, name string, args ...string) ([]byte, error) {
	path := filepath.Join(home, ".local", "share", "mise", "shims") + ":" + os.Getenv("PATH")
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "env", append([]string{"PATH=" + path, name}, args...)...)
	return cmd.Output()
}

func miseToolAuthenticated(home, name string, args ...string) bool {
	out, err := miseToolOutput(home, name, args...)
	return err == nil && len(bytes.TrimSpace(out)) > 0
}

func firebaseAuthenticated(home string) bool {
	out, err := miseToolOutput(home, "firebase", "login:list", "--json")
	if err != nil {
		return false
	}
	var status struct {
		Result []json.RawMessage `json:"result"`
	}
	return json.Unmarshal(out, &status) == nil && len(status.Result) > 0
}

func miseToolLoginStep(home, label, command string, args ...string) step {
	path := filepath.Join(home, ".local", "share", "mise", "shims") + ":" + os.Getenv("PATH")
	argv := []string{"env", "PATH=" + path, command}
	return terminalStep(label, append(argv, args...)...)
}

func cloudLoginSteps(home string) []step {
	return []step{
		skipWhen(miseToolLoginStep(home, "Autenticar na AWS", "aws", "login"), func() bool {
			return miseToolAuthenticated(home, "aws", "sts", "get-caller-identity", "--output", "text")
		}),
		skipWhen(miseToolLoginStep(home, "Autenticar no Google Cloud", "gcloud", "auth", "login"), func() bool {
			return miseToolAuthenticated(home, "gcloud", "auth", "list", "--filter=status:ACTIVE", "--format=value(account)")
		}),
		skipWhen(miseToolLoginStep(home, "Autenticar no Railway", "railway", "login"), func() bool { return miseToolAuthenticated(home, "railway", "whoami") }),
		skipWhen(miseToolLoginStep(home, "Autenticar no Firebase", "firebase", "login"), func() bool { return firebaseAuthenticated(home) }),
		skipWhen(miseToolLoginStep(home, "Autenticar no Azure", "az", "login"), func() bool { return miseToolAuthenticated(home, "az", "account", "show", "--output", "json") }),
	}
}

func npmLoginStep(home string) step {
	return skipWhen(miseToolLoginStep(home, "Autenticar no npm", "npm", "login"), func() bool { return miseToolAuthenticated(home, "npm", "whoami") })
}

func rustupToolchainStep() step {
	return terminalStep("Instalar toolchain Rust estável", "rustup", "default", "stable")
}

func betterStackInstallStep() step {
	return terminalStep("Instalar Better Stack CLI", "sh", "-c", betterStackInstallCommand)
}

func betterStackLoginStep(home string) step {
	path := filepath.Join(home, ".local", "bin") + ":" + os.Getenv("PATH")
	return skipWhen(terminalStep("Autenticar no Better Stack", "env", "PATH="+path, "bs", "auth", "init"), func() bool {
		config, err := os.ReadFile(filepath.Join(home, ".config", "bs", "config.toml"))
		return err == nil && regexp.MustCompile(`(?m)^uptime_token\s*=\s*"[^"]+"`).Match(config)
	})
}

func rustToolPath(home, name string) (string, error) {
	cargoPath := filepath.Join(home, ".cargo", "bin", name)
	if info, err := os.Stat(cargoPath); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
		return cargoPath, nil
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s não encontrado após instalar o Rust", name)
	}
	return path, nil
}

func verifyRustTools(home string) error {
	for _, name := range []string{"rustc", "cargo"} {
		path, err := rustToolPath(home, name)
		if err != nil {
			return err
		}
		if output, err := exec.Command(path, "--version").CombinedOutput(); err != nil {
			return fmt.Errorf("validar %s: %s: %w", name, string(output), err)
		}
	}
	return nil
}

func developmentEnvironmentSteps(dotfiles, home string, packageSteps []step) []step {
	steps := miseSetupSteps(dotfiles)
	steps = append(steps, skipWhen(developmentToolsInstallStep(dotfiles), func() bool {
		out, err := exec.Command("mise", "--cd", dotfiles, "ls", "--missing", "--current", "--no-header").Output()
		return err == nil && len(bytes.TrimSpace(out)) == 0
	}))
	steps = append(steps, cloudLoginSteps(home)...)
	steps = append(steps, packageSteps...)
	steps = append(steps, fishConfigStep(dotfiles, home))
	return append(steps,
		skipWhen(rustupToolchainStep(), func() bool {
			out, err := exec.Command("rustup", "toolchain", "list").Output()
			return err == nil && strings.Contains(string(out), "stable-")
		}),
		nativeStep("Verificar rustc e cargo", func() error {
			return verifyRustTools(home)
		}),
		skipWhen(betterStackInstallStep(), func() bool {
			return executableFile(filepath.Join(home, ".local", "bin", "bs")) || commandSucceeds(nil, "bs", "--version")
		}),
		betterStackLoginStep(home),
		npmLoginStep(home),
	)
}

func developmentEnvironmentJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	steps := developmentEnvironmentSteps(dotfiles, home, ensurePkgs(developmentPacmanPackages...))
	return job{
		title: "Configurar Ambiente de Desenvolvimento",
		steps: steps,
	}
}
