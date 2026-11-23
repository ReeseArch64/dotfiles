package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type miseLogin struct {
	page  screen
	title string
	tool  string
	argv  []string
}

var miseLogins = []miseLogin{
	{screenLoginAWS, "AWS", "awscli", []string{"aws", "login"}},
	{screenLoginGCloud, "Google Cloud", "gcloud", []string{"gcloud", "auth", "login"}},
	{screenLoginAzure, "Azure", "azure-cli", []string{"az", "login"}},
	{screenLoginRailway, "Railway", "railway", []string{"railway", "login"}},
	{screenLoginGitHub, "GitHub", "gh", []string{"gh", "auth", "login"}},
	{screenLoginFirebase, "Firebase", "firebase", []string{"firebase", "login"}},
	{screenLoginCloudflare, "Cloudflare CLI", "npm:cf", []string{"cf", "auth", "login"}},
	{screenLoginWrangler, "Cloudflare Wrangler", "wrangler", []string{"wrangler", "login"}},
	{screenLoginVercel, "Vercel", "npm:vercel", []string{"vercel", "login"}},
	{screenLoginNeon, "Neon", "npm:neon", []string{"neon", "auth"}},
	{screenLoginNPM, "npm", "node", []string{"npm", "login"}},
	{screenLoginPNPM, "pnpm", "pnpm", []string{"pnpm", "login"}},
	{screenLoginUV, "uv", "uv", []string{"uv", "auth", "login"}},
}

func miseLoginForScreen(page screen) (miseLogin, bool) {
	for _, login := range miseLogins {
		if login.page == page {
			return login, true
		}
	}
	return miseLogin{}, false
}

func (login miseLogin) commandLine() string {
	line := strings.Join(login.argv, " ")
	if login.page == screenLoginUV {
		line += " <serviço>"
	}
	return line
}

func runMiseLogin(device profile, name string) error {
	if device.sshKeyName() == "" || os.Geteuid() == 0 {
		return errors.New("execute o login pelo menu Programação do Desktop ou Laptop como usuário normal")
	}
	for _, login := range miseLogins {
		if login.argv[0] != name {
			continue
		}
		console := sshConsole{input: bufio.NewReader(os.Stdin), output: os.Stdout, commands: systemSSHCommands()}
		printActionHeader()
		err := login.run(&console)
		if err != nil {
			fmt.Fprintln(console.output, err)
		}
		_, _ = console.prompt("\nEnter para voltar ao menu...")
		return err
	}
	return errors.New("CLI de login inválida")
}

func (login miseLogin) run(console *sshConsole) error {
	args, err := login.command(console.commands)
	if err != nil {
		return err
	}
	args = append(args, login.argv[1:]...)
	if login.page == screenLoginUV {
		service, err := console.prompt("Domínio ou URL do serviço: ")
		if err != nil {
			return err
		}
		if service == "" {
			return errors.New("informe o domínio ou a URL do serviço")
		}
		args = append(args, "--", service)
	}
	if err := console.commands.interactive(args...); err != nil {
		return fmt.Errorf("login de %s: %w", login.title, err)
	}
	return nil
}

func (login miseLogin) command(commands sshCommands) ([]string, error) {
	name := login.argv[0]
	prefix := []string{"env", "MISE_AUTO_INSTALL=0", "mise", "--cd", "/"}
	find := func(tool ...string) string {
		args := append(prefix, "--quiet", "which", name)
		path, err := commands.capture("", append(args, tool...)...)
		path = strings.TrimSpace(path)
		if err != nil || !filepath.IsAbs(path) || strings.ContainsAny(path, "\r\n") {
			return ""
		}
		return path
	}
	if path := find(); path != "" {
		return append(prefix, "exec", "--", path), nil
	}
	version, err := commands.capture("", append(prefix, "--quiet", "latest", "--installed", login.tool)...)
	if fields := strings.Fields(version); err == nil && len(fields) == 1 {
		tool := login.tool + "@" + fields[0]
		if path := find("--tool", tool); path != "" {
			return append(prefix, "exec", tool, "--", path), nil
		}
	}
	if path := loginExecutableOnPath(name); path != "" {
		return []string{path}, nil
	}
	return nil, fmt.Errorf("executável %s não encontrado no Mise nem no PATH. Verifique Programação → Mise", name)
}

func loginExecutableOnPath(name string) string {
	mise, _ := exec.LookPath("mise")
	miseInfo, _ := os.Stat(mise)
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if !filepath.IsAbs(dir) {
			continue
		}
		path, err := exec.LookPath(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		// Mise shims can remain on PATH after a tool stops being active.
		info, err := os.Stat(path)
		if err == nil && !os.SameFile(info, miseInfo) {
			return path
		}
	}
	return ""
}
