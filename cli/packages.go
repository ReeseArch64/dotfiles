package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

const yayRepository = "https://aur.archlinux.org/yay.git"

var essentialPackages = []string{"base-devel", "curl", "wget", "rate-mirrors"}
var terminalPackages = []string{
	"bat", "eza", "btop", "fzf", "ripgrep", "fd", "jq", "yq", "zoxide",
	"ncdu", "tealdeer", "wl-clipboard", "atuin", "starship", "yazi", "sshfs", "glow",
	"ffmpeg", "mpv", "yt-dlp", "scrcpy", "android-tools", "hurl", "hyperfine", "just", "cmake", "ninja",
}

type packageInstaller struct {
	console *sshConsole
	which   func(string) (string, error)
	build   func(string) error
}

func runPackageAction(profile profile, action string) error {
	if profile.sshKeyName() == "" || os.Geteuid() == 0 {
		return errors.New("execute a instalação pelo menu Desktop ou Laptop como usuário normal")
	}
	if action == "setup-turing" && profile.prefix != "desktop" {
		return errors.New("Turing Smart Screen está disponível no perfil Desktop")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(home) {
		return errors.New("a pasta pessoal precisa ter um caminho absoluto")
	}
	console := sshConsole{home: home, input: bufio.NewReader(os.Stdin), output: os.Stdout, commands: systemSSHCommands()}
	installer := packageInstaller{console: &console, which: exec.LookPath, build: buildYay}
	printActionHeader()
	err = installer.run(action, "/usr/lib/modules")
	if err == nil {
		fmt.Fprintln(console.output, "Instalação concluída.")
	} else {
		fmt.Fprintln(console.output, err)
	}
	_, _ = console.prompt("\nEnter para voltar ao menu...")
	return err
}

func (installer packageInstaller) run(action, modules string) error {
	var official []string
	var aur []string
	var executable string
	switch action {
	case "essential-tools":
		official = essentialPackages
	case "terminal-tools":
		official = terminalPackages
		aur = []string{"viddy", "ventoy", "scc", "usql-bin", "mprocs"}
	case "install-posting":
		return installer.installPosting()
	case "install-lunarvim":
		if installer.userToolInstalled("lvim") {
			return nil
		}
		executable = findMise(installer.console.home, installer.which)
		official = []string{"git", "curl", "base-devel", "ripgrep", "fd"}
		if executable == "" {
			official = append([]string{"mise"}, official...)
		}
	case "install-kitty":
		official = []string{"kitty"}
	case "install-artcraft-launcher":
		aur = []string{"artcraft-launcher-bin"}
	case "install-zed":
		official = []string{"zed"}
	case "setup-obsidian":
		if !installer.installed("obsidian") {
			aur = []string{"obsidian-bin"}
		}
	case "setup-turing":
		return installer.setupTuring()
	case "setup-discord":
		return installer.setupDiscord()
	case "install-firefox":
		official = []string{"firefox", "xdg-utils"}
	case "install-rust":
		executable = installer.rustupExecutable()
		if executable == "" {
			official = []string{"rustup"}
		}
	case "drivers":
		headers, err := installer.kernelHeaders(modules)
		if err != nil {
			return err
		}
		official = append(headers, "base-devel", "vulkan-tools", "mesa-utils")
	case "proton-pass":
		if err := installer.checkProtonPassFirefox(); err != nil {
			return err
		}
		aur = []string{"proton-pass-cli-bin"}
	case "install-yay":
	default:
		return errors.New("ação de instalação inválida")
	}

	if len(official) > 0 {
		fmt.Fprintln(installer.console.output, "Pacotes dos repositórios:", strings.Join(official, ", "))
	}
	if len(aur) != 0 {
		fmt.Fprintln(installer.console.output, "Pacotes do AUR:", strings.Join(aur, ", "))
	}
	if action == "install-yay" || len(aur) != 0 {
		fmt.Fprintln(installer.console.output, "Se necessário, instalar git e base-devel e compilar yay em ~/.aur/yay.")
	}
	if action == "proton-pass" {
		fmt.Fprintln(installer.console.output, "Iniciar login com pass-cli login após a instalação.")
	}
	if action == "install-firefox" {
		fmt.Fprintln(installer.console.output, "Definir o Firefox como navegador padrão.")
	}
	if action == "install-rust" {
		fmt.Fprintln(installer.console.output, "Usar a toolchain estável como padrão e verificar rustc e cargo.")
	}
	if action == "setup-obsidian" {
		fmt.Fprintln(installer.console.output, "Aplicar aparência e snippet Noctalia em ~/.obsidian.")
	}
	if err := installer.console.confirm("Digite CONFIRMO para instalar: ", "CONFIRMO"); err != nil {
		return err
	}
	if action == "install-yay" {
		return installer.ensureYay()
	}
	aur = slices.DeleteFunc(aur, installer.installed)
	if len(aur) != 0 {
		if err := installer.ensureYay(); err != nil {
			return err
		}
	}
	if err := installer.console.ensurePackages(official...); err != nil {
		return err
	}
	if len(aur) != 0 {
		if err := installer.console.commands.interactive(append([]string{"yay", "-S", "--needed", "--"}, aur...)...); err != nil {
			return err
		}
	}
	if action == "proton-pass" {
		if err := installer.console.commands.interactive("pass-cli", "login"); err != nil {
			return fmt.Errorf("executar pass-cli login: %w", err)
		}
	}
	switch action {
	case "install-firefox":
		return setFirefoxDefault(installer.console.commands)
	case "setup-obsidian":
		return setupObsidian(installer.console)
	case "install-rust":
		if executable == "" {
			executable = installer.rustupExecutable()
			if executable == "" {
				return errors.New("executável do Rustup indisponível após a instalação")
			}
		}
		return configureRust(installer.console, executable)
	case "install-lunarvim":
		if executable == "" {
			executable = findMise(installer.console.home, installer.which)
			if executable == "" {
				return errors.New("executável do Mise indisponível após a instalação")
			}
		}
		return installLunarVim(installer.console, executable)
	}
	return nil
}

func (installer packageInstaller) userToolInstalled(name string, candidates ...string) bool {
	candidates = append(candidates, filepath.Join(installer.console.home, ".local", "bin", name))
	if findExecutable(installer.which, name, "", candidates...) != "" {
		fmt.Fprintln(installer.console.output, name, "já está instalado.")
		return true
	}
	return false
}

func (installer packageInstaller) rustupExecutable() string {
	return findExecutable(installer.which, "rustup", "",
		filepath.Join(installer.console.home, ".cargo", "bin", "rustup"), "/usr/bin/rustup")
}

func setFirefoxDefault(commands sshCommands) error {
	if err := commands.interactive("env", "-u", "BROWSER", "xdg-settings", "set", "default-web-browser", "firefox.desktop"); err != nil {
		return fmt.Errorf("definir o Firefox como navegador padrão: %w", err)
	}
	return nil
}

func (installer packageInstaller) checkProtonPassFirefox() error {
	if _, err := installer.which("firefox"); err != nil {
		return errors.New("instale o Firefox antes de instalar o Proton Pass")
	}
	path := filepath.Join(installer.console.home, ".config", "mozilla")
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return errors.New("abra o Firefox para inicializar ~/.config/mozilla antes de instalar o Proton Pass")
	}
	if err != nil {
		return fmt.Errorf("verificar ~/.config/mozilla: %w", err)
	}
	if !info.IsDir() {
		return errors.New("~/.config/mozilla precisa ser um diretório")
	}
	return nil
}

func (installer packageInstaller) installed(name string) bool {
	_, err := installer.console.commands.capture("", "pacman", "-Q", name)
	return err == nil
}

func (installer packageInstaller) kernelHeaders(modules string) ([]string, error) {
	entries, err := os.ReadDir(modules)
	if err != nil {
		return nil, err
	}
	var headers []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		image := filepath.Join(modules, entry.Name(), "vmlinuz")
		if _, err := os.Stat(image); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		owner, err := installer.console.commands.capture("", "pacman", "-Qqo", "--", image)
		if err != nil {
			return nil, fmt.Errorf("identificar o pacote do kernel %s: %w", image, err)
		}
		names := strings.Fields(owner)
		if len(names) != 1 || strings.HasPrefix(names[0], "-") {
			return nil, fmt.Errorf("pacote do kernel %s não identificado", image)
		}
		header := names[0] + "-headers"
		if !slices.Contains(headers, header) {
			headers = append(headers, header)
		}
	}
	if len(headers) == 0 {
		return nil, errors.New("nenhum kernel instalado foi identificado em " + modules)
	}
	for _, header := range headers {
		if !installer.installed(header) {
			if _, err := installer.console.commands.capture("", "pacman", "-Si", "--", header); err != nil {
				return nil, fmt.Errorf("headers indisponíveis para instalação (%s): %w", header, err)
			}
		}
	}
	return headers, nil
}

func aurDirectory(home, sdk string) (string, error) {
	root := filepath.Join(home, ".aur")
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(root, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return "", err
		}
		info, err = os.Lstat(root)
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		destination, err := os.Readlink(root)
		if err != nil {
			return "", err
		}
		if !filepath.IsAbs(destination) {
			destination = filepath.Join(home, destination)
		}
		if filepath.Clean(destination) == filepath.Join(sdk, "Aur") {
			return root, sdkRealDirectoryPath(filepath.Clean(destination))
		}
	}
	if !info.IsDir() {
		return "", fmt.Errorf("diretório AUR inválido: %s", root)
	}
	return root, nil
}

func (installer packageInstaller) ensureYay() error {
	if _, err := installer.which("yay"); err == nil {
		return nil
	}
	root, err := aurDirectory(installer.console.home, sdkRoot)
	if err != nil {
		return err
	}
	directory, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	unlock, err := lockDirectory(directory, "instalação do yay")
	if err != nil {
		return err
	}
	defer unlock()
	if _, err := installer.which("yay"); err == nil {
		return nil
	}
	if _, err := installer.which("git"); err != nil {
		if err := installer.console.ensurePackages("git"); err != nil {
			return err
		}
		if _, err := installer.which("git"); err != nil {
			return fmt.Errorf("git não está disponível após a instalação: %w", err)
		}
	}
	target := filepath.Join(root, "yay")
	_, err = os.Lstat(target)
	missing := errors.Is(err, os.ErrNotExist)
	if err != nil && !missing {
		return err
	}
	if !missing {
		if err := installer.checkYayClone(target); err != nil {
			return err
		}
	}
	if err := installer.console.ensurePackages("base-devel"); err != nil {
		return err
	}
	if missing {
		if err := installer.console.commands.interactive("git", "clone", "--", yayRepository, target); err != nil {
			return err
		}
		if err := installer.checkYayClone(target); err != nil {
			return err
		}
	}
	if err := installer.build(target); err != nil {
		return err
	}
	if _, err := installer.which("yay"); err != nil {
		return fmt.Errorf("yay não está disponível após a instalação: %w", err)
	}
	return nil
}

func (installer packageInstaller) checkYayClone(target string) error {
	for _, directory := range []string{target, filepath.Join(target, ".git")} {
		if info, err := os.Lstat(directory); err != nil || !info.IsDir() {
			return fmt.Errorf("clone do yay inválido: %s", target)
		}
	}
	capture := installer.console.commands.capture
	url, err := capture("", "git", "-C", target, "config", "--local", "--get", "remote.origin.url")
	if err != nil || strings.TrimSpace(url) != yayRepository {
		return fmt.Errorf("o clone em %s precisa usar o repositório oficial do yay", target)
	}
	status, err := capture("", "git", "-C", target, "status", "--porcelain", "--untracked-files=all")
	if err != nil {
		return err
	}
	if status != "" {
		return fmt.Errorf("o clone do yay contém alterações locais: %s", target)
	}
	return sshRegular(filepath.Join(target, "PKGBUILD"), false)
}

func buildYay(target string) error {
	staging, err := os.MkdirTemp(filepath.Dir(target), ".dotfiles-yay-build-*")
	if err != nil {
		return err
	}
	defer forceRemoveAll(staging)
	// Keep build outputs away from the reusable Git checkout.
	cmd := exec.Command("makepkg", "-si", "BUILDDIR="+staging, "PKGDEST="+staging,
		"SRCDEST="+staging, "SRCPKGDEST="+staging, "LOGDEST="+staging)
	cmd.Dir = target
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
