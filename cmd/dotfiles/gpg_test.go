package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestGPGGenerateStepsOrder(t *testing.T) {
	steps := gpgGenerateSteps("/dotfiles/setup/laptop", "/home/test", []step{{label: "Instalar gnupg"}})
	want := []string{
		"Validar Git e identidade do dispositivo", "Instalar gnupg", "Preparar geração GPG",
		"Gerar chave GPG com senha", "Identificar chave gerada",
		"Exibir chave pública para GitHub, GitLab e Gitea", "Configurar user.signingkey",
	}
	if got := jobStepLabels(job{steps: steps}); !slices.Equal(got, want) {
		t.Fatalf("passos inesperados: %v", got)
	}
	if steps[3].cmd == nil || steps[5].cmd == nil || steps[6].run == nil {
		t.Fatal("geração e exibição devem receber o terminal; configuração deve ser nativa")
	}
	if _, err := steps[4].run(); err == nil {
		t.Fatal("status ausente não pode identificar chave")
	}
	if _, err := steps[6].run(); err == nil {
		t.Fatal("não pode configurar assinatura sem chave gerada")
	}
}

func TestGPGGenerateStepsRequiresConfiguredGit(t *testing.T) {
	steps := gpgGenerateSteps(t.TempDir(), t.TempDir(), nil)
	if _, err := steps[0].run(); err == nil || !strings.Contains(err.Error(), "configure o Git") {
		t.Fatalf("Git não configurado deve impedir geração: %v", err)
	}
}

func TestParseGPGCreatedFingerprint(t *testing.T) {
	fingerprint := "0123456789ABCDEF0123456789ABCDEF01234567"
	for _, kind := range []string{"P", "B"} {
		got, err := parseGPGCreatedFingerprint("[GNUPG:] PROGRESS primegen X 1 0\n[GNUPG:] KEY_CREATED " + kind + " " + fingerprint + "\n")
		if err != nil || got != fingerprint {
			t.Fatalf("fingerprint inesperado: %q, %v", got, err)
		}
	}
	for _, output := range []string{"", "[GNUPG:] KEY_CREATED P invalid", "[GNUPG:] KEY_CREATED S " + fingerprint, "KEY_CREATED P " + fingerprint} {
		if _, err := parseGPGCreatedFingerprint(output); err == nil {
			t.Fatalf("status inválido aceito: %s", output)
		}
	}
}

func TestGPGDeviceIdentityUsesSelectedProfileAndHostname(t *testing.T) {
	forEachRepositoryProfile(t, func(t *testing.T, setupDir string) {
		got, err := gpgDeviceIdentity(setupDir, "notebook-test")
		if err != nil || got != "Alan Guerra (notebook-test) <dev@reesearch64.tech>" {
			t.Fatalf("identidade inesperada: %q, %v", got, err)
		}
	})
}

func TestGPGDeviceIdentityRejectsInvalidConfiguration(t *testing.T) {
	for _, content := range []string{"[user]\nname = Test\n", "[user]\nname =\nemail = test@example.invalid\n", "[user]\nname = Test <Injected>\nemail = test@example.invalid\n"} {
		setup := t.TempDir()
		if err := os.Mkdir(filepath.Join(setup, "git"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(setup, "git", ".gitconfig"), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := gpgDeviceIdentity(setup, "notebook"); err == nil {
			t.Fatal("identidade inválida aceita")
		}
	}
}

func TestGPGGenerationCancellationRemovesStatus(t *testing.T) {
	status := filepath.Join(t.TempDir(), "status")
	if err := os.WriteFile(status, nil, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-c", gpgGenerateScript, "test", status, "Test (device) <test@example.invalid>")
	cmd.Stdin = strings.NewReader("")
	if err := cmd.Run(); err == nil {
		t.Fatal("entrada cancelada deve falhar")
	}
	if _, err := os.Stat(status); !os.IsNotExist(err) {
		t.Fatalf("arquivo temporário não foi removido: %v", err)
	}
}

func TestGPGGeneratesProtectedLocalKeyAndConfiguresSigning(t *testing.T) {
	if _, err := exec.LookPath("gpg"); err != nil {
		t.Skip("gpg não está disponível")
	}
	gnupgHome, err := os.MkdirTemp("", "dotfiles-gpg-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(gnupgHome)
	t.Setenv("GNUPGHOME", gnupgHome)
	defer exec.Command("gpgconf", "--kill", "gpg-agent").Run()
	status := filepath.Join(t.TempDir(), "status")
	const passphrase = "synthetic-test-passphrase"
	identity := "Dotfiles Test (test-device) <dotfiles-test@example.invalid>"
	cmd := exec.Command("bash", "-c", gpgGenerateScript, "test", status, identity)
	cmd.Stdin = strings.NewReader("\n\nwrong\ndifferent\n" + passphrase + "\n" + passphrase + "\n")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("gerar chave temporária: %s: %v", output, err)
	}
	if strings.Contains(string(output), passphrase) || !strings.Contains(string(output), "não pode ficar vazia") || !strings.Contains(string(output), "não coincidem") {
		t.Fatalf("senha exposta ou validação ausente: %s", output)
	}
	created, err := os.ReadFile(status)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := parseGPGCreatedFingerprint(string(created))
	if err != nil {
		t.Fatal(err)
	}
	show := exec.Command("bash", "-c", gpgShowPublicScript, "test", fingerprint)
	show.Stdin = strings.NewReader("\n")
	public, err := show.CombinedOutput()
	if err != nil {
		t.Fatalf("exibir chave: %s: %v", public, err)
	}
	if !strings.Contains(string(public), "-----BEGIN PGP PUBLIC KEY BLOCK-----") || !strings.Contains(string(public), "-----END PGP PUBLIC KEY BLOCK-----") || strings.Contains(string(public), "PRIVATE KEY") || !strings.Contains(string(public), fingerprint) {
		t.Fatalf("exportação pública inesperada: %s", public)
	}
	// Esvazia o cache do agente antes de tentar assinar sem senha.
	if err := exec.Command("gpgconf", "--kill", "gpg-agent").Run(); err != nil {
		t.Fatal(err)
	}
	message := filepath.Join(t.TempDir(), "message")
	if err := os.WriteFile(message, []byte("test signature"), 0600); err != nil {
		t.Fatal(err)
	}
	unsigned := exec.Command("gpg", "--batch", "--pinentry-mode", "loopback", "--passphrase-fd", "0", "--local-user", fingerprint, "--detach-sign", message)
	unsigned.Stdin = strings.NewReader("\n")
	if err := unsigned.Run(); err == nil {
		t.Fatal("chave gerada não deve assinar sem senha")
	}
	sign := exec.Command("gpg", "--batch", "--yes", "--pinentry-mode", "loopback", "--passphrase-fd", "0", "--local-user", fingerprint, "--detach-sign", message)
	sign.Stdin = strings.NewReader(passphrase + "\n")
	if output, err := sign.CombinedOutput(); err != nil {
		t.Fatalf("assinar com senha: %s: %v", output, err)
	}
	setup, home := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(setup, "git"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath(setup, "git", ".gitconfig"), []byte("[user]\n signingkey =\n name = Test\n email = test@example.invalid\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := applyGitSigningKey(setup, home, fingerprint); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{configPath(setup, "git", ".gitconfig"), filepath.Join(home, ".gitconfig")} {
		got, err := readGitSigningKey(path)
		if err != nil || got != fingerprint {
			t.Fatalf("assinatura inesperada em %s: %q, %v", path, got, err)
		}
	}
}

func TestDevelopmentMenuPlacesGPGAfterGit(t *testing.T) {
	model := newModel("/dotfiles")
	model.screen = screenDevelopment
	items := model.items()
	for i := range items {
		if items[i].title == "Git" {
			if i+1 >= len(items) || items[i+1].title != "GPG" {
				t.Fatal("GPG deve aparecer imediatamente após Git")
			}
			return
		}
	}
	t.Fatal("Git ausente no menu Desenvolvimento")
}

func TestGitStatusReadyForGPG(t *testing.T) {
	ready := gitStatus{links: []gitLink{{ok: true}, {ok: true}}}
	if !ready.readyForGPG() {
		t.Fatal("esperava Git pronto")
	}
	ready.missing = []string{"git"}
	if ready.readyForGPG() {
		t.Fatal("Git com pacote ausente não pode liberar GPG")
	}
	ready.missing = nil
	ready.links[0].ok = false
	if ready.readyForGPG() {
		t.Fatal("Git com link ausente não pode liberar GPG")
	}
}
