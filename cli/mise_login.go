package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type miseLogin struct {
	page  screen
	title string
	argv  []string
}

var miseLogins = []miseLogin{
	{screenLoginAWS, "AWS", []string{"aws", "login"}},
	{screenLoginGCloud, "Google Cloud", []string{"gcloud", "auth", "login"}},
	{screenLoginAzure, "Azure", []string{"az", "login"}},
	{screenLoginRailway, "Railway", []string{"railway", "login"}},
	{screenLoginGitHub, "GitHub", []string{"gh", "auth", "login"}},
	{screenLoginFirebase, "Firebase", []string{"firebase", "login"}},
	{screenLoginCloudflare, "Cloudflare CLI", []string{"cf", "auth", "login"}},
	{screenLoginWrangler, "Cloudflare Wrangler", []string{"wrangler", "login"}},
	{screenLoginVercel, "Vercel", []string{"vercel", "login"}},
	{screenLoginNeon, "Neon", []string{"neon", "auth"}},
	{screenLoginNPM, "npm", []string{"npm", "login"}},
	{screenLoginPNPM, "pnpm", []string{"pnpm", "login"}},
	{screenLoginUV, "uv", []string{"uv", "auth", "login"}},
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
		config, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		if !filepath.IsAbs(config) {
			return errors.New("o diretório de configuração precisa ter um caminho absoluto")
		}
		console := sshConsole{input: bufio.NewReader(os.Stdin), output: os.Stdout, commands: systemSSHCommands()}
		printActionHeader()
		err = login.run(&console, filepath.Join(config, "mise", "config.toml"))
		if err != nil {
			fmt.Fprintln(console.output, err)
		}
		_, _ = console.prompt("\nEnter para voltar ao menu...")
		return err
	}
	return errors.New("CLI de login inválida")
}

func (login miseLogin) run(console *sshConsole, config string) error {
	prefix := []string{"env", "MISE_GLOBAL_CONFIG_FILE=" + config, "MISE_AUTO_INSTALL=0", "mise", "--cd", "/"}
	path, err := console.commands.capture("", append(prefix, "--quiet", "which", login.argv[0])...)
	if err != nil {
		return fmt.Errorf("localizar %s com o Mise. Verifique Programação → Mise: %w", login.title, err)
	}
	executable := strings.TrimSpace(path)
	if !filepath.IsAbs(executable) || strings.ContainsRune(executable, '\n') {
		return fmt.Errorf("o Mise não retornou um caminho válido para %s", login.title)
	}
	args := append(prefix, "exec", "--", executable)
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
