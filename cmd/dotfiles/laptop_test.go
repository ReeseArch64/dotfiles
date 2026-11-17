package main

import (
	"os"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLaptopSubtitle(t *testing.T) {
	m := newModelForDevice("/repo", deviceLaptop)
	m.width, m.height = 120, 40
	view := m.View()
	if !strings.Contains(view, "Omarchy · Wayland · by ReeseArch64 · Laptop · v1.0.0") {
		t.Fatalf("subtítulo do laptop incorreto: %s", view)
	}
	for _, old := range []string{"CachyOS", "Noctalia Shell", "Laptop (Omarchy Linux)", "v0.15.0"} {
		if strings.Contains(view, old) {
			t.Fatalf("texto antigo na interface: %s", old)
		}
	}
}

func TestLaptopFirewallMenuAndJob(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range requiredSSHIdentityFiles(deviceLaptop) {
		if err := os.WriteFile(filepath.Join(home, ".ssh", name), []byte("ssh-ed25519 YWJj desktop\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for name, contents := range map[string]string{"config": "Host *\n IdentityFile ~/.ssh/id_laptop\n", "authorized_keys": "ssh-ed25519 YWJj desktop\n"} {
		if err := os.WriteFile(filepath.Join(home, ".ssh", name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m := newModelForDevice("/repo", deviceLaptop)
	account, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	m.ssh = sshStatus{listening: true, service: unitState{"active", "enabled"}, cfg: map[string]string{
		"port": "22", "permitrootlogin": "no", "passwordauthentication": "no", "kbdinteractiveauthentication": "no",
		"pubkeyauthentication": "yes", "authenticationmethods": "publickey", "allowusers": account.Username,
		"authorizedkeysfile": ".ssh/authorized_keys", "authorizedkeyscommand": "none", "trustedusercakeys": "none",
	}}
	if got := itemTitles(m.items()); !slices.Equal(got, []string{"SSH", "Firewall", "Aparência", "Sair"}) {
		t.Fatalf("menu inesperado: %v", got)
	}
	m.screen = screenFirewall
	m.fw = fwStatus{port: "22", lan: "192.168.1.0/24"}
	items := m.items()
	if got := itemTitles(items); !slices.Equal(got, []string{"Configurar Firewall"}) {
		t.Fatalf("menu firewall inesperado: %v", got)
	}
	labels := jobStepLabels(items[0].job())
	for _, want := range []string{"Verificar pré-requisito SSH", "Entrada: bloquear por padrão", "Saída: permitir por padrão", "Habilitar ufw.service no boot", "Ativar UFW", "Liberar 22/tcp para 192.168.1.0/24"} {
		if !slices.Contains(labels, want) {
			t.Fatalf("passo ausente: %s", want)
		}
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "authorized_keys"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if m.items()[0].disabled {
		t.Fatal("firewall bloqueado com authorized_keys vazio")
	}
	m.ssh.listening = false
	if !m.items()[0].disabled {
		t.Fatal("firewall disponível sem porta SSH escutando")
	}
	m.ssh.listening = true
	if err := os.Remove(filepath.Join(home, ".ssh", "authorized_keys")); err != nil {
		t.Fatal(err)
	}
	// A revalidação nativa permanece antes de todas as alterações de firewall.
	job := items[0].job()
	for _, step := range job.steps {
		if step.label == "Revalidar porta SSH e LAN" {
			if _, err := step.run(); err == nil {
				t.Fatal("job não revalidou o pré-requisito SSH")
			}
		}
	}
	locked := m.items()[0]
	if !locked.disabled || locked.job != nil {
		t.Fatal("firewall deveria bloquear sem chave autorizada")
	}
}

func TestLaptopFirewallLockedWithoutSSH(t *testing.T) {
	m := newModelForDevice("/repo", deviceLaptop)
	mainItem := m.items()[1]
	if mainItem.title != "Firewall" || mainItem.desc != "Requer SSH configurado" || !mainItem.disabled {
		t.Fatal("firewall deve aparecer desativado e sem cadeado")
	}
	nextMain, cmdMain := m.selectItem(mainItem)
	if cmdMain != nil || nextMain.(model).screen != screenMain {
		t.Fatal("firewall desativado abriu a tela")
	}
	m.cursor[screenMain] = 1
	if strings.Contains(m.viewMenu(60, 0), "▌") {
		t.Fatal("item desativado não deve receber destaque de seleção")
	}
	m.screen = screenFirewall
	it := m.items()[0]
	if !it.disabled || it.job != nil {
		t.Fatal("firewall disponível sem SSH")
	}
	next, cmd := m.selectItem(it)
	if cmd != nil || next.(model).screen != screenFirewall {
		t.Fatal("item bloqueado foi executado")
	}
}

func TestLaptopSSHMenus(t *testing.T) {
	m := newModelForDevice("/repo", deviceLaptop)
	m.screen = screenSSH
	if got := itemTitles(m.items()); !slices.Equal(got, []string{"Configurar SSH", "Chaves autorizadas"}) {
		t.Fatalf("menu SSH inesperado: %v", got)
	}
	m.screen = screenSSHKeys
	if got := itemTitles(m.items()); !slices.Equal(got, []string{"Adicionar chave pública", "Visualizar chaves atuais", "Remover chave pública"}) {
		t.Fatalf("menu de chaves inesperado: %v", got)
	}
	if got := requiredSSHIdentityFiles(deviceLaptop); !slices.Equal(got, []string{"id_laptop", "id_laptop.pub"}) {
		t.Fatalf("laptop ainda exige identidades de plataformas: %v", got)
	}
	job := m.laptopSSHJob("configure", "Configurar SSH")
	command := job.steps[len(job.steps)-1].cmd()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(command.Args, []string{executable, "--laptop-ssh", "configure"}) {
		t.Fatalf("configuração deve usar o próprio binário Go: %v", command.Args)
	}
}

func TestLaptopPlatformRequiresOmarchy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "os-release")
	for _, id := range []string{"omarchy", "arch", "cachyos", "ubuntu"} {
		if err := os.WriteFile(path, []byte("ID="+id+"\n"), 0644); err != nil {
			t.Fatal(err)
		}
		err := checkPlatformForDevice(deviceLaptop, path, func(string) (string, error) {
			t.Fatal("laptop não deve exigir ferramentas de desktop")
			return "", nil
		})
		if (err == nil) != (id == "omarchy") {
			t.Fatalf("ID=%s: %v", id, err)
		}
	}
}
