package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
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
	{screenLoginGitLab, "GitLab", "glab", []string{"glab", "auth", "login"}},
	{screenLoginHeroku, "Heroku", "heroku", []string{"heroku", "login"}},
	{screenLoginRender, "Render", "github:render-oss/cli", []string{"render", "login"}},
	{screenLoginSentry, "Sentry", "github:getsentry/cli", []string{"sentry", "auth", "login"}},
	{screenLoginMaple, "Maple", "github:MapleTechLabs/maple", []string{"maple", "auth", "login"}},
	{screenLoginAxiom, "Axiom", "github:axiomhq/cli", []string{"axiom", "auth", "login", "--auto-login=false"}},
	{screenLoginLinear, "Linear", "github:schpet/linear-cli", []string{"linear", "auth", "login"}},
	{screenLoginFirebase, "Firebase", "firebase", []string{"firebase", "login"}},
	{screenLoginCloudflare, "Cloudflare CLI", "npm:cf", []string{"cf", "auth", "login"}},
	{screenLoginWrangler, "Cloudflare Wrangler", "wrangler", []string{"wrangler", "login"}},
	{screenLoginVercel, "Vercel", "npm:vercel", []string{"vercel", "login"}},
	{screenLoginNeon, "Neon", "npm:neon", []string{"neon", "auth"}},
	{screenLoginExpo, "Expo (EAS)", "npm:eas-cli", []string{"eas", "login"}},
	{screenLoginClaude, "Claude Code", "claude", []string{"claude", "auth", "login"}},
	{screenLoginCodex, "Codex", "codex", []string{"codex", "login"}},
	{screenLoginCopilot, "GitHub Copilot CLI", "copilot", []string{"copilot", "login"}},
	{screenLoginGemini, "Gemini CLI", "gemini", []string{"gemini"}},
	{screenLoginAntigravity, "Antigravity CLI", "antigravity-cli", []string{"agy", "acp", "--login"}},
	{screenLoginNPM, "npm", "node", []string{"npm", "login"}},
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
	if login.page == screenLoginGemini {
		return "Use /auth para entrar e /quit para voltar"
	}
	return strings.Join(login.argv, " ")
}

func runMiseLogin(device profile, name string, console sshConsole) error {
	if device.sshKeyName() == "" || os.Geteuid() == 0 {
		return errors.New("execute o login pelo menu Programação do Desktop ou Laptop como usuário normal")
	}
	for _, login := range miseLogins {
		if login.argv[0] != name {
			continue
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		console.home = home
		return login.run(&console)
	}
	return errors.New("CLI de login inválida")
}

func (login miseLogin) run(console *sshConsole) error {
	args, err := login.command(console, exec.LookPath)
	if err != nil {
		return err
	}
	if login.page == screenLoginGemini {
		workspace, err := os.MkdirTemp("", "dotfiles-gemini-login-*")
		if err != nil {
			return fmt.Errorf("criar pasta temporária para o Gemini: %w", err)
		}
		defer os.RemoveAll(workspace)
		// Keep Mise resolution outside projects and give the Gemini session an empty workspace.
		args = slices.Insert(args, len(args)-1, "env", "--chdir="+workspace)
		fmt.Fprintln(console.output, "Use /auth para escolher a autenticação e /quit para voltar ao menu.")
	}
	args = append(args, login.argv[1:]...)
	if err := console.commands.interactive(args...); err != nil {
		return fmt.Errorf("login de %s: %w", login.title, err)
	}
	return nil
}

func (login miseLogin) command(console *sshConsole, lookup func(string) (string, error)) ([]string, error) {
	name := login.argv[0]
	mise := findMise(console.home, lookup)
	if args := installedMiseTool(console.commands, mise, name, login.tool); args != nil {
		return args, nil
	}
	if path := findExecutable(lookup, name, mise); path != "" {
		return []string{path}, nil
	}
	return nil, fmt.Errorf("executável %s não encontrado no Mise nem no PATH. Verifique Programação → Mise", name)
}
