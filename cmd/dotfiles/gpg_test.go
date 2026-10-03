package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGPGImportPathsUsesDotfilesDirectory(t *testing.T) {
	dir := t.TempDir()
	public := filepath.Join(dir, "minha_chave_publica.asc")
	private := filepath.Join(dir, "minha_chave_privada.asc")
	keys := map[string]string{
		public:  "-----BEGIN PGP PUBLIC KEY BLOCK-----\npublic\n",
		private: "-----BEGIN PGP PRIVATE KEY BLOCK-----\nprivate\n",
	}
	for path, content := range keys {
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}

	paths, err := loadGPGImportPaths(dir)
	if err != nil {
		t.Fatal(err)
	}
	if paths.public != public || paths.private != private {
		t.Fatalf("caminhos inesperados: %#v", paths)
	}
}

func TestGPGImportPathsRejectsMissingOrInvalidKeys(t *testing.T) {
	tests := []struct {
		name    string
		public  string
		private string
		want    string
	}{
		{"chave pública ausente", "", "-----BEGIN PGP PRIVATE KEY BLOCK-----\nprivate\n", "acessar chave pública"},
		{"chave privada ausente", "-----BEGIN PGP PUBLIC KEY BLOCK-----\npublic\n", "", "acessar chave privada"},
		{"chave pública inválida", "não é uma chave", "-----BEGIN PGP PRIVATE KEY BLOCK-----\nprivate\n", "bloco OpenPGP válido"},
		{"chave privada inválida", "-----BEGIN PGP PUBLIC KEY BLOCK-----\npublic\n", "não é uma chave", "bloco OpenPGP válido"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.public != "" {
				if err := os.WriteFile(filepath.Join(dir, "minha_chave_publica.asc"), []byte(tt.public), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if tt.private != "" {
				if err := os.WriteFile(filepath.Join(dir, "minha_chave_privada.asc"), []byte(tt.private), 0600); err != nil {
					t.Fatal(err)
				}
			}
			_, err := loadGPGImportPaths(dir)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("esperava erro contendo %q, recebeu %v", tt.want, err)
			}
		})
	}
}

func TestGPGImportStepsInstallPackageBeforeImports(t *testing.T) {
	install := step{label: "Instalar gnupg"}
	steps := gpgImportSteps("/dotfiles", gpgImportPaths{"/media/public.asc", "/media/private.asc"}, errors.New("parar"), []step{install})
	want := []string{"Validar Git e arquivos .asc", "Instalar gnupg", "Importar chave pública", "Importar chave privada", "Configurar user.signingkey"}
	if len(steps) != len(want) {
		t.Fatalf("quantidade de passos: recebeu %d, esperava %d", len(steps), len(want))
	}
	for i, label := range want {
		if steps[i].label != label {
			t.Fatalf("passo %d: recebeu %q, esperava %q", i, steps[i].label, label)
		}
	}
	for i, path := range []string{"/media/public.asc", "/media/private.asc"} {
		args := steps[i+2].cmd().Args
		got := args[len(args)-3:]
		wantArgs := []string{"gpg", "--import", path}
		for j := range wantArgs {
			if got[j] != wantArgs[j] {
				t.Fatalf("argumento %d da importação %d: recebeu %q, esperava %q", j, i, got[j], wantArgs[j])
			}
		}
	}
	if steps[len(steps)-1].run == nil {
		t.Fatal("configuração de user.signingkey deve ser um passo nativo")
	}
}

func TestParseGPGSecretFingerprintUsesPrimarySecretKey(t *testing.T) {
	primary := "0123456789ABCDEF0123456789ABCDEF01234567"
	subkey := "89ABCDEF0123456789ABCDEF0123456789ABCDEF"
	output := strings.Join([]string{
		"sec:-:255:22:89ABCDEF01234567:0:0:::::::",
		"fpr:::::::::" + primary + ":",
		"uid:::::::::Test User:",
		"ssb:-:255:18:0123456789ABCDEF:0:0:::::::",
		"fpr:::::::::" + subkey + ":",
	}, "\n")

	got, err := parseGPGSecretFingerprint(output)
	if err != nil {
		t.Fatal(err)
	}
	if got != primary {
		t.Fatalf("fingerprint inesperado: %s", got)
	}
}

func TestParseGPGSecretFingerprintRejectsPublicKey(t *testing.T) {
	output := "pub:-:255:22:89ABCDEF01234567:0:0:::::::\nfpr:::::::::0123456789ABCDEF0123456789ABCDEF01234567:"
	if _, err := parseGPGSecretFingerprint(output); err == nil {
		t.Fatal("esperava erro para chave sem material privado")
	}
}

func TestSigningKeyFromPrivateFile(t *testing.T) {
	if _, err := exec.LookPath("gpg"); err != nil {
		t.Skip("gpg não está disponível")
	}
	gnupgHome := filepath.Join(t.TempDir(), "gnupg")
	if err := os.Mkdir(gnupgHome, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GNUPGHOME", gnupgHome)
	defer exec.Command("gpgconf", "--kill", "gpg-agent").Run()
	identity := "Dotfiles Test <dotfiles-test@example.invalid>"
	generate := exec.Command("gpg", "--batch", "--passphrase", "", "--quick-generate-key", identity, "ed25519", "sign", "1d")
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("gerar chave temporária: %s: %v", output, err)
	}
	privateKey := filepath.Join(t.TempDir(), "private.asc")
	exported, err := exec.Command("gpg", "--batch", "--armor", "--export-secret-keys", identity).Output()
	if err != nil {
		t.Fatalf("exportar chave temporária: %v", err)
	}
	if err := os.WriteFile(privateKey, exported, 0600); err != nil {
		t.Fatal(err)
	}

	fingerprint, err := signingKeyFromPrivateFile(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if !validGPGFingerprint(fingerprint) {
		t.Fatalf("fingerprint inválido: %q", fingerprint)
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
