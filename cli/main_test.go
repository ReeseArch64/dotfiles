package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestMenuSelectsSSHKeyByHostname(t *testing.T) {
	tests := []struct {
		hostname string
		keyName  string
		subtitle string
	}{
		{"desktop-alan", "id_desktop", "CachyOS · Niri · Noctalia Shell · by ReeseArch64 · Desktop"},
		{"LAPTOP-ALAN", "id_laptop", "CachyOS · Niri · Noctalia Shell · by ReeseArch64 · Laptop"},
		{"homeprod-alan", "", "Ubuntu · by ReeseArch64 · HomeProd"},
		{"homelab", "", "Ubuntu · by ReeseArch64 · Homelab"},
		{"HOMELAB-ALAN", "", "Ubuntu · by ReeseArch64 · Homelab"},
		{"desktop-laptop", "", "Sem suporte · by ReeseArch64"},
		{"homelab-homeprod", "", "Sem suporte · by ReeseArch64"},
		{"notebook", "", "Sem suporte · by ReeseArch64"},
	}
	for _, tt := range tests {
		t.Run(tt.hostname, func(t *testing.T) {
			m := newMenu(tt.hostname)
			m.width = 100
			if !strings.Contains(m.View(), tt.subtitle+" · v"+version) {
				t.Fatalf("subtítulo incorreto: %q", m.subtitle())
			}
			if got := m.profile.sshKeyName(); got != tt.keyName {
				t.Fatalf("nome da chave = %q, esperado %q", got, tt.keyName)
			}
			want := []string{"Sistema Operacional", "Segurança", "Navegador Web", "Programação", "CasaOS", "Agentes de IA", "Programas", "Backup", "Sair"}
			if items := menuTitles(m.items()); !slices.Equal(items, want) {
				t.Fatalf("opções = %v, esperado %v", items, want)
			}
			if tt.keyName != "" {
				programming, _ := m.selectItem(m.items()[3])
				programmingMenu := programming.(model)
				wantProgramming := []string{"Git", "Mise"}
				if m.profile.prefix == "desktop" {
					wantProgramming = append(wantProgramming, "Docker")
				}
				wantProgramming = append(wantProgramming, "Login das CLIs", "Instalar Rust")
				if got := menuTitles(programmingMenu.items()); !slices.Equal(got, wantProgramming) {
					t.Fatalf("menu de programação = %v", got)
				}
				gitMenu, _ := programmingMenu.selectItem(programmingMenu.items()[0])
				if got := gitMenu.(model).items(); len(got) != 1 || got[0].action != "setup-git" {
					t.Fatalf("menu Git = %v", got)
				}
				miseMenu, _ := programmingMenu.selectItem(programmingMenu.items()[1])
				miseItems := miseMenu.(model).items()
				if len(miseItems) != 1 {
					t.Fatalf("menu Mise = %v", miseItems)
				}
				miseCommand, err := actionCommand(miseItems[0].action, miseItems[0].title)
				if err != nil || !slices.Equal(miseCommand.Args[1:], []string{"--setup-mise"}) {
					t.Fatalf("ação do Mise: %v, %v", miseCommand, err)
				}
				rust := programmingMenu.items()[len(wantProgramming)-1]
				rustCommand, err := actionCommand(rust.action, rust.title)
				if rust.disabled || err != nil || !slices.Equal(rustCommand.Args[1:], []string{"--packages", "install-rust"}) {
					t.Fatalf("ação do Rust: %v, %v", rustCommand, err)
				}
				browser, _ := m.selectItem(m.items()[2])
				browserItems := browser.(model).items()
				if got := menuTitles(browserItems); !slices.Equal(got, []string{"Backup Completo", "Instalar Firefox"}) {
					t.Fatalf("menu de navegador = %v", got)
				}
				firefox := browserItems[1]
				firefoxCommand, err := actionCommand(firefox.action, firefox.title)
				if firefox.disabled || err != nil || !slices.Equal(firefoxCommand.Args[1:], []string{"--packages", "install-firefox"}) {
					t.Fatalf("ação do Firefox: %v, %v", firefoxCommand, err)
				}
				system, _ := m.selectItem(m.items()[0])
				osMenu := system.(model)
				if got := menuTitles(osMenu.items()); !slices.Equal(got, []string{
					"Mouse personalizado", "Migrar pastas para /mnt/sdk", "Fish", "Fastfetch", "Servidor", "Papéis de parede", "Noctalia", "Armazenamento e projetos", "Niri",
				}) {
					t.Fatalf("menu do sistema = %v", got)
				}
				if height := lipgloss.Height(osMenu.View()); height != osMenu.height {
					t.Fatalf("menu do sistema ocupa %d linhas em terminal com %d", height, osMenu.height)
				}
				niri := osMenu.items()[8]
				niriCommand, err := actionCommand(niri.action, niri.title)
				if niri.disabled != (m.profile.prefix != "desktop") || err != nil || !slices.Equal(niriCommand.Args[1:], []string{"--setup-niri"}) {
					t.Fatalf("ação do Niri: %v, %v", niriCommand, err)
				}
				osMenu.osID = "cachyos"
				storage, _ := osMenu.handleKey("8")
				items := storage.(model).items()
				if got := menuTitles(items); !slices.Equal(got, []string{"Migrar pastas para /mnt/storage", "Configurar ~/Projects"}) {
					t.Fatalf("menu Armazenamento e projetos = %v", got)
				}
				for index, flag := range []string{"--storage-links", "--setup-workspaces"} {
					item := items[index]
					command, err := actionCommand(item.action, item.title)
					if item.disabled || err != nil || !slices.Equal(command.Args[1:], []string{flag}) {
						t.Fatalf("ação de armazenamento %s: %v, %v", item.title, command, err)
					}
				}
				programs, _ := m.handleKey("7")
				programMenu := programs.(model)
				if programMenu.screen != screenPrograms {
					t.Fatal("atalho não abriu Programas")
				}
				if got := menuTitles(programMenu.items()); !slices.Equal(got, []string{
					"Ferramentas Essenciais", "Ferramentas de Terminal", "Drivers", "Terminal", "Instalar ArtCraft Launcher", "Aplicativos pré-instalados",
				}) {
					t.Fatalf("menu Programas = %v", got)
				}
				launcher := programMenu.items()[4]
				launcherCommand, err := actionCommand(launcher.action, launcher.title)
				if launcher.disabled || err != nil || !slices.Equal(launcherCommand.Args[1:], []string{"--packages", "install-artcraft-launcher"}) {
					t.Fatalf("ação do ArtCraft Launcher: %v, %v", launcherCommand, err)
				}
				wallpapers := osMenu.items()[5]
				wallpapersCommand, err := actionCommand(wallpapers.action, wallpapers.title)
				if wallpapers.disabled || err != nil || !slices.Equal(wallpapersCommand.Args[1:], []string{"--wallpapers"}) {
					t.Fatalf("ação de papéis de parede: %v, %v", wallpapersCommand, err)
				}
				noctalia := osMenu.items()[6]
				noctaliaCommand, err := actionCommand(noctalia.action, noctalia.title)
				if noctalia.disabled || err != nil || !slices.Equal(noctaliaCommand.Args[1:], []string{"--setup-noctalia"}) {
					t.Fatalf("ação do Noctalia: %v, %v", noctaliaCommand, err)
				}
				terminal, _ := programMenu.handleKey("4")
				terminalItems := terminal.(model).items()
				if got := menuTitles(terminalItems); !slices.Equal(got, []string{"Instalar Kitty"}) {
					t.Fatalf("submenu Terminal = %v", got)
				}
				command, err := actionCommand(terminalItems[0].action, terminalItems[0].title)
				if err != nil || !slices.Equal(command.Args[1:], []string{"--packages", "install-kitty"}) {
					t.Fatalf("ação de instalação do Kitty: %v, %v", command, err)
				}
				for index, app := range []string{"fish", "fastfetch"} {
					child, _ := osMenu.selectItem(osMenu.items()[index+2])
					items := child.(model).items()
					if len(items) != 1 {
						t.Fatalf("submenu de %s: %v", app, items)
					}
					command, err := actionCommand(items[0].action, items[0].title)
					if err != nil || !slices.Equal(command.Args[1:], []string{"--terminal-config", app}) {
						t.Fatalf("ação de %s: %v, %v", app, command, err)
					}
				}
				for index, actions := range [][]string{{"essential-tools"}, {"install-yay", "terminal-tools", "install-posting", "install-lunarvim"}, {"drivers"}} {
					child, _ := programMenu.selectItem(programMenu.items()[index])
					items := child.(model).items()
					if len(items) != len(actions) {
						t.Fatalf("submenu de ferramentas %d: %v", index, items)
					}
					for index, action := range actions {
						command, err := actionCommand(items[index].action, items[index].title)
						if err != nil || !slices.Equal(command.Args[1:], []string{"--packages", action}) {
							t.Fatalf("ação de ferramentas %s: %v, %v", action, command, err)
						}
					}
				}
				security, _ := m.selectItem(m.items()[1])
				securityMenu := security.(model)
				if got := menuTitles(securityMenu.items()); !slices.Equal(got, []string{"SSH", "Firewall", "Proton Pass"}) {
					t.Fatalf("menu Segurança = %v", got)
				}
				if securityMenu.items()[2].action != "proton-pass" {
					t.Fatal("ação do Proton Pass ausente")
				}
				submenu, _ := securityMenu.selectItem(securityMenu.items()[0])
				ssh := submenu.(model)
				want := []string{"Configurar SSH", "Criar chave SSH", "Chaves autorizadas"}
				if got := menuTitles(ssh.items()); !slices.Equal(got, want) {
					t.Fatalf("submenu SSH = %v, esperado %v", got, want)
				}
				keys, _ := ssh.selectItem(ssh.items()[2])
				want = []string{"Adicionar chave pública", "Visualizar chaves atuais", "Remover chave pública"}
				if got := menuTitles(keys.(model).items()); !slices.Equal(got, want) {
					t.Fatalf("submenu de chaves = %v, esperado %v", got, want)
				}
			}
		})
	}
}

func TestMenuStructureAndAvailability(t *testing.T) {
	for _, tt := range []struct {
		hostname string
		main     []string
		security []string
		ssh      []string
	}{
		{"desktop", []string{"Sistema Operacional", "Segurança", "Navegador Web", "Programação", "Agentes de IA", "Programas", "Backup", "Sair"}, []string{"SSH", "Firewall", "Proton Pass"}, []string{"Configurar SSH", "Criar chave SSH", "Chaves autorizadas"}},
		{"laptop", []string{"Sistema Operacional", "Segurança", "Navegador Web", "Programação", "Agentes de IA", "Programas", "Backup", "Sair"}, []string{"SSH", "Firewall", "Proton Pass"}, []string{"Configurar SSH", "Criar chave SSH", "Chaves autorizadas"}},
		{"homelab", []string{"Sistema Operacional", "Segurança", "CasaOS", "Programas", "Sair"}, []string{"SSH", "Firewall"}, []string{"Configurar SSH"}},
		{"homeprod", []string{"Sair"}, nil, nil},
		{"unknown", []string{"Sair"}, nil, nil},
	} {
		t.Run(tt.hostname, func(t *testing.T) {
			menu := newMenu(tt.hostname)
			menu.status.sshReady = true
			for page := screenMain; page < screenJob; page++ {
				menu.screen = page
				items := menu.items()
				var enabled []string
				for i, item := range items {
					if !item.disabled {
						enabled = append(enabled, item.title)
					}
					items[i].disabled = false
				}
				if !slices.Equal(items, screenItems(page, menu.profile)) {
					t.Fatalf("estrutura diferente em %s: %v", screenTitles[page], items)
				}
				switch page {
				case screenMain:
					if !slices.Equal(enabled, tt.main) {
						t.Fatalf("menu principal habilitado: %v", enabled)
					}
				case screenSecurity:
					if !slices.Equal(enabled, tt.security) {
						t.Fatalf("menu Segurança habilitado: %v", enabled)
					}
				case screenSSH:
					if !slices.Equal(enabled, tt.ssh) {
						t.Fatalf("menu SSH habilitado: %v", enabled)
					}
				}
			}
		})
	}
}

func TestMenuNavigationSkipsUnavailableItems(t *testing.T) {
	menu := newMenu("homelab")
	if got := menu.menuCursor(menu.items()); got != 0 {
		t.Fatalf("seleção inicial deveria ser Sistema Operacional: %d", got)
	}
	if next, command := menu.handleKey("3"); command != nil || next.(model).screen != screenMain {
		t.Fatal("atalho abriu Navegador Web desabilitado")
	}
	next, _ := menu.handleKey("down")
	menu = next.(model)
	next, _ = menu.handleKey("down")
	menu = next.(model)
	if got := menu.menuCursor(menu.items()); got != 4 {
		t.Fatalf("seleção seguinte deveria ser CasaOS: %d", got)
	}
	next, _ = menu.handleKey("enter")
	if next.(model).screen != screenCasaOS {
		t.Fatal("Enter não abriu CasaOS")
	}
	menu = newMenu("homeprod")
	if got := menu.menuCursor(menu.items()); got != 8 {
		t.Fatalf("seleção inicial deveria ser Sair: %d", got)
	}
	if next, command := menu.handleKey("2"); command != nil || next.(model).screen != screenMain {
		t.Fatal("atalho abriu Segurança desabilitada")
	}
}

func TestSDKMigrationOnlyOnCachyOSWorkstations(t *testing.T) {
	for _, tt := range []struct {
		host, release string
		allowed       bool
	}{
		{"desktop", "ID=cachyos\n", true},
		{"laptop", "ID=\"cachyos\"\n", true},
		{"laptop", "ID=omarchy\nID_LIKE=arch\n", false},
		{"desktop", "ID=ubuntu\n", false},
		{"homelab", "ID=cachyos\n", false},
		{"homeprod", "ID=ubuntu\n", false},
		{"desktop", "", false},
	} {
		m := newMenu(tt.host)
		m.osID = osReleaseID(tt.release)
		m.screen = screenSystem
		if m.items()[1].disabled == tt.allowed || m.profile.canMigrateSDK(m.osID) != tt.allowed {
			t.Fatalf("host %s, os-release %q: migração SDK deveria estar habilitada=%t", tt.host, tt.release, tt.allowed)
		}
	}
}

func TestLegacyMenuCardsAndCursor(t *testing.T) {
	m := newMenu("laptop-alan")
	m.width, m.height = 100, 35
	m.status = networkStatus{loaded: true, sshInstalled: true, sshActive: true, sshEnabled: true, port: "22", password: "no", root: "no"}
	security, _ := m.handleKey("2")
	m = security.(model)
	view := ansi.Strip(m.View())
	for _, want := range []string{"Menu principal › Segurança", "▌ SSH", "▌ Configurar SSH e gerenciar chaves autorizadas", "Firewall 2", "Requer SSH configurado", "1-9 atalho"} {
		if !strings.Contains(view, want) {
			t.Fatalf("menu principal deve conter %q", want)
		}
	}
	blocked, cmd := m.handleKey("2")
	if blocked.(model).screen != screenSecurity || cmd != nil {
		t.Fatal("o firewall deve ficar desabilitado antes de configurar SSH")
	}
	m.status.sshReady = true
	firewall, _ := m.handleKey("2")
	if view := ansi.Strip(firewall.(model).View()); !strings.Contains(view, "Segurança › Firewall") || !strings.Contains(view, "LAN") {
		t.Fatalf("cartão de firewall ausente: %s", view)
	}
	ssh, _ := m.handleKey("1")
	if view := ansi.Strip(ssh.(model).View()); !strings.Contains(view, "Segurança › SSH") || !strings.Contains(view, "rodando (sshd.service)") {
		t.Fatalf("cartão de SSH ausente: %s", view)
	}
	keys, _ := ssh.(model).handleKey("3")
	if view := ansi.Strip(keys.(model).View()); !strings.Contains(view, "Segurança › SSH ›") ||
		!strings.Contains(view, "Chaves autorizadas") {
		t.Fatalf("caminho do submenu de chaves ausente: %s", view)
	}
	job, cmd := keys.(model).handleKey("1")
	if cmd == nil || job.(model).screen != screenJob {
		t.Fatal("selecionar ação deve abrir uma tela de execução")
	}
	done, _ := job.Update(actionResult{})
	if !strings.Contains(ansi.Strip(done.View()), "Concluído") {
		t.Fatal("tela de execução não mostra resultado")
	}
	back, _ := done.(model).handleJobKey("q")
	back, _ = back.(model).handleKey("esc")
	if back.(model).screen != screenSSH || back.(model).cursor[screenSSH] != 2 {
		t.Fatal("retorno deve preservar a posição no menu SSH")
	}
}

func menuTitles(items []menuItem) []string {
	var titles []string
	for _, item := range items {
		titles = append(titles, item.title)
	}
	return titles
}
