package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAzureLoginFailureDoesNotBlockDevelopment(t *testing.T) {
	continued := false
	steps := cloudLoginSteps(t.TempDir())
	azure := steps[len(steps)-1]
	r := newJobRun(job{steps: []step{azure, nativeStep("Continuar configuração", func() error {
		continued = true
		return nil
	})}})
	r.cur = 0
	r.states[0] = stepRunning
	command := r.handle(stepDoneMsg{err: errors.New("a conta Azure ainda não tem assinatura")})
	if r.failed || r.done || command == nil || r.states[0] != stepIgnored {
		t.Fatalf("falha do Azure interrompeu o fluxo: failed=%v done=%v states=%v", r.failed, r.done, r.states)
	}
	message, ok := command().(stepDoneMsg)
	if !ok || message.err != nil || !continued {
		t.Fatalf("próxima etapa não executou: %#v continued=%v", message, continued)
	}
}

func TestAzureOptionalLoginFailsWithoutPrompt(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, ".local", "share", "mise", "shims")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestExecutable(t, bin, "az", `printf 'sem assinatura\n' >&2; exit 1`)
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("PATH", "/usr/bin:/bin")
	azure := cloudLoginSteps(home)[4]
	if !azure.optional {
		t.Fatal("login Azure precisa ser opcional")
	}
	out, err := azure.cmd().CombinedOutput()
	if err == nil || !strings.Contains(string(out), "sem assinatura") || !strings.Contains(string(out), "continuando a configuração") {
		t.Fatalf("resultado inesperado: err=%v out=%s", err, out)
	}
	if strings.Contains(string(out), "Pressione qualquer tecla") {
		t.Fatal("login opcional aguardou tecla após falhar")
	}
	if cloudLoginSteps(home)[0].optional {
		t.Fatal("outros logins não devem ficar opcionais")
	}
}

func TestCloudAndNpmLoginCheckExistingSessions(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(home, ".local", "share", "mise", "shims")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"aws", "gcloud", "railway", "az", "npm"} {
		writeTestExecutable(t, bin, name, `[ "$*" = "login" ] && exit 9; printf 'account\n'`)
	}
	writeTestExecutable(t, bin, "firebase", `printf '{"result":[{"email":"test@example.com"}]}'`)
	for _, current := range append(cloudLoginSteps(home), npmLoginStep(home)) {
		if !current.skip() {
			t.Fatalf("sessão existente não detectada: %s", current.label)
		}
	}
	writeTestExecutable(t, bin, "npm", `exit 1`)
	if npmLoginStep(home).skip() {
		t.Fatal("sessão npm ausente foi ignorada")
	}
}

func TestInstalledRustWithoutDefaultStillConfiguresToolchain(t *testing.T) {
	bin := t.TempDir()
	writeTestExecutable(t, bin, "rustup", `case "$*" in
  "toolchain list") printf 'stable-x86_64-unknown-linux-gnu\n' ;;
  "default") printf 'no default toolchain configured\n'; exit 1 ;;
esac`)
	writeTestExecutable(t, bin, "rustc", `printf 'error: rustup could not choose a version of rustc to run\n' >&2; exit 1`)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	home := t.TempDir()
	if err := verifyRustTools(home); err == nil || !strings.Contains(err.Error(), "rustup could not choose") {
		t.Fatalf("falha original não reproduzida: %v", err)
	}
	for _, current := range developmentEnvironmentSteps(t.TempDir(), home, nil) {
		if current.label == "Instalar toolchain Rust estável" {
			if current.skip() {
				t.Fatal("toolchain instalada sem default não pode ser ignorada")
			}
			return
		}
	}
	t.Fatal("etapa da toolchain ausente")
}

func TestDevelopmentSkipsInstalledMiseToolsAndRust(t *testing.T) {
	bin := t.TempDir()
	writeTestExecutable(t, bin, "mise", `exit 0`)
	writeTestExecutable(t, bin, "rustup", `printf 'stable-x86_64-unknown-linux-gnu (default)\n'`)
	for _, name := range []string{"rustc", "cargo"} {
		writeTestExecutable(t, bin, name, `printf '1.0.0\n'`)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	steps := developmentEnvironmentSteps(t.TempDir(), t.TempDir(), nil)
	for _, current := range steps {
		if current.label == "Instalar ferramentas configuradas via mise" || current.label == "Instalar toolchain Rust estável" {
			if !current.skip() {
				t.Fatalf("instalação existente não ignorada: %s", current.label)
			}
		}
	}
	writeTestExecutable(t, bin, "mise", `printf 'node 22\n'`)
	writeTestExecutable(t, bin, "rustup", `exit 1`)
	for _, current := range steps {
		if current.label == "Instalar ferramentas configuradas via mise" || current.label == "Instalar toolchain Rust estável" {
			if current.skip() {
				t.Fatalf("instalação ausente ignorada: %s", current.label)
			}
		}
	}
}

func TestBetterStackSkipsInstallerAndConfiguredLogin(t *testing.T) {
	home := t.TempDir()
	binary := filepath.Join(home, ".local", "bin", "bs")
	if err := os.MkdirAll(filepath.Dir(binary), 0755); err != nil {
		t.Fatal(err)
	}
	writeTestExecutable(t, filepath.Dir(binary), "bs", `exit 0`)
	steps := developmentEnvironmentSteps(t.TempDir(), home, nil)
	for _, current := range steps {
		if current.label == "Instalar Better Stack CLI" && !current.skip() {
			t.Fatal("CLI instalada novamente")
		}
	}
	if err := os.Remove(binary); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(home, ".config", "bs", "config.toml")
	if err := os.MkdirAll(filepath.Dir(config), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("[auth]\nuptime_token = \"secret\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if !betterStackLoginStep(home).skip() {
		t.Fatal("configuração existente não detectada")
	}
	if err := os.WriteFile(config, []byte("[auth]\nuptime_token = \"\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if betterStackLoginStep(home).skip() {
		t.Fatal("token vazio não deve ignorar login")
	}
}

func TestDevelopmentPacmanPackagesIncludeFishRustupAndTk(t *testing.T) {
	want := []string{"fish", "rustup", "tk"}
	if !slices.Equal(developmentPacmanPackages, want) {
		t.Fatalf("pacotes do ambiente inesperados: %v", developmentPacmanPackages)
	}
}

func TestDevelopmentToolsInstallStepUsesMiseConfig(t *testing.T) {
	current := developmentToolsInstallStep("/tmp/dotfiles")
	want := []string{"mise", "--cd", "/tmp/dotfiles", "install"}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando mise inesperado: %v", got)
	}
}

func TestCloudLoginStepsUseMiseShims(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	steps := cloudLoginSteps("/tmp/home")
	want := []struct {
		label string
		args  []string
	}{
		{"Autenticar na AWS", []string{"env", "PATH=/tmp/home/.local/share/mise/shims:/usr/bin", "aws", "login"}},
		{"Autenticar no Google Cloud", []string{"env", "PATH=/tmp/home/.local/share/mise/shims:/usr/bin", "gcloud", "auth", "login"}},
		{"Autenticar no Railway", []string{"env", "PATH=/tmp/home/.local/share/mise/shims:/usr/bin", "railway", "login"}},
		{"Autenticar no Firebase", []string{"env", "PATH=/tmp/home/.local/share/mise/shims:/usr/bin", "firebase", "login"}},
		{"Autenticar no Azure", []string{"env", "PATH=/tmp/home/.local/share/mise/shims:/usr/bin", "az", "login"}},
	}
	if len(steps) != len(want) {
		t.Fatalf("quantidade de autenticações inesperada: %d", len(steps))
	}
	for i, expected := range want {
		if steps[i].label != expected.label {
			t.Fatalf("rótulo inesperado: %s", steps[i].label)
		}
		if got := commandTail(t, steps[i], len(expected.args)); !slices.Equal(got, expected.args) {
			t.Fatalf("comando de autenticação inesperado para %s: %v", expected.label, got)
		}
	}
}

func TestNpmLoginStepUsesMiseShim(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	current := npmLoginStep("/tmp/home")
	want := []string{"env", "PATH=/tmp/home/.local/share/mise/shims:/usr/bin", "npm", "login"}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando npm login inesperado: %v", got)
	}
}

func TestBetterStackInstallStepUsesOfficialInstaller(t *testing.T) {
	current := betterStackInstallStep()
	want := []string{"sh", "-c", betterStackInstallCommand}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando de instalação inesperado: %v", got)
	}
}

func TestBetterStackLoginStepUsesLocalBin(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	current := betterStackLoginStep("/tmp/home")
	want := []string{"env", "PATH=/tmp/home/.local/bin:/usr/bin", "bs", "auth", "init"}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando de autenticação inesperado: %v", got)
	}
}

func TestRustupToolchainStepInstallsStable(t *testing.T) {
	current := rustupToolchainStep()
	want := []string{"rustup", "default", "stable"}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando rustup inesperado: %v", got)
	}
}

func TestVerifyRustToolsRunsRustcAndCargo(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "rust.log")
	for _, name := range []string{"rustc", "cargo"} {
		writeTestExecutable(t, bin, name, `printf '%s %s\n' "$(basename "$0")" "$*" >> "$RUST_LOG"
printf '%s 1.0.0\n' "$(basename "$0")"`)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("RUST_LOG", logPath)

	if err := verifyRustTools(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(content)
	for _, want := range []string{"rustc --version", "cargo --version"} {
		if !strings.Contains(log, want) {
			t.Fatalf("verificação %q ausente:\n%s", want, log)
		}
	}
}

func TestDevelopmentEnvironmentJobConfiguresAllTools(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	steps := developmentEnvironmentSteps("/tmp/dotfiles", home, []step{{label: "Instalar fish, rustup, tk"}})
	labels := jobStepLabels(job{steps: steps})
	for _, want := range []string{
		"Symlink ~/.config/mise/config.toml",
		"Instalar ferramentas configuradas via mise",
		"Autenticar na AWS",
		"Autenticar no Google Cloud",
		"Autenticar no Railway",
		"Autenticar no Firebase",
		"Autenticar no Azure",
		"Instalar fish, rustup, tk",
		"Copiar configuração para ~/.config/fish/config.fish",
		"Instalar toolchain Rust estável",
		"Verificar rustc e cargo",
		"Instalar Better Stack CLI",
		"Autenticar no Better Stack",
		"Autenticar no npm",
	} {
		if !slices.Contains(labels, want) {
			t.Fatalf("passo %q ausente: %v", want, labels)
		}
	}
	if slices.Index(labels, "Instalar ferramentas configuradas via mise") > slices.Index(labels, "Autenticar na AWS") {
		t.Fatalf("ferramentas do Mise devem ser instaladas antes das autenticações: %v", labels)
	}
	if slices.Index(labels, "Autenticar no Azure") > slices.Index(labels, "Instalar fish, rustup, tk") {
		t.Fatalf("autenticações cloud devem ocorrer antes das instalações do Pacman: %v", labels)
	}
	if slices.Index(labels, "Instalar fish, rustup, tk") > slices.Index(labels, "Copiar configuração para ~/.config/fish/config.fish") {
		t.Fatalf("Fish deve ser instalado antes de copiar sua configuração: %v", labels)
	}
	if slices.Index(labels, "Instalar fish, rustup, tk") > slices.Index(labels, "Instalar toolchain Rust estável") {
		t.Fatalf("rustup deve ser instalado antes da toolchain: %v", labels)
	}
	if slices.Index(labels, "Instalar Better Stack CLI") > slices.Index(labels, "Autenticar no Better Stack") {
		t.Fatalf("Better Stack deve ser instalado antes da autenticação: %v", labels)
	}
	if labels[len(labels)-1] != "Autenticar no npm" {
		t.Fatalf("último passo inesperado: %s", labels[len(labels)-1])
	}
}
