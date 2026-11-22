package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type screen int

const (
	screenMain screen = iota
	screenSystem
	screenEssentials
	screenTerminalTools
	screenDrivers
	screenFish
	screenFastfetch
	screenTerminal
	screenProgramming
	screenGit
	screenMise
	screenSecurity
	screenSSH
	screenSSHKeys
	screenFirewall
	screenCasaOS
	screenWebBrowser
	screenJob
)

var screenTitles = map[screen]string{
	screenMain:          "Menu principal",
	screenSystem:        "Sistema Operacional",
	screenEssentials:    "Ferramentas Essenciais",
	screenTerminalTools: "Ferramentas de Terminal",
	screenDrivers:       "Drivers",
	screenFish:          "Fish",
	screenFastfetch:     "Fastfetch",
	screenTerminal:      "Terminal",
	screenProgramming:   "Programação",
	screenGit:           "Git",
	screenMise:          "Mise",
	screenSecurity:      "Segurança",
	screenSSH:           "SSH",
	screenSSHKeys:       "Chaves autorizadas",
	screenFirewall:      "Firewall",
	screenCasaOS:        "CasaOS",
	screenWebBrowser:    "Navegador Web",
}

type tickMsg time.Time
type actionResult struct{ err error }
type statusResult struct {
	request int
	status  networkStatus
}

type model struct {
	profile       profile
	start, now    time.Time
	width, height int
	screen        screen
	stack         []screen
	cursor        [screenJob]int
	job           *jobRun
	status        networkStatus
	statusRequest int
}

func newMenu(hostname string) model {
	now := time.Now()
	return model{profile: identifyProfile(hostname), start: now, now: now, width: 80, height: 24}
}

func tick() tea.Cmd {
	return tea.Tick(time.Second/30, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) Init() tea.Cmd {
	if m.profile.sshKeyName() == "" && m.profile.prefix != "homelab" {
		return tick()
	}
	return tea.Batch(tick(), m.loadStatus())
}

type menuItem struct {
	title, desc string
	action      string
	goTo        screen
	quit        bool
	disabled    bool
}

func (m model) items() []menuItem {
	items := screenItems(m.screen)
	for i := range items {
		items[i].disabled = !m.menuItemAvailable(items[i])
		if items[i].goTo == screenFirewall && items[i].disabled && m.profile.sshKeyName() != "" {
			items[i].desc = "Requer SSH configurado"
		}
	}
	return items
}

func (m model) menuItemAvailable(item menuItem) bool {
	workstation := m.profile.sshKeyName() != ""
	homelab := m.profile.prefix == "homelab"
	switch {
	case item.quit:
		return true
	case item.goTo == screenCasaOS || item.action == "homelab-casaos":
		return homelab
	case item.goTo == screenSecurity || item.goTo == screenSSH || item.action == "configure":
		return workstation || homelab
	case item.goTo == screenFirewall || item.action == "firewall":
		return homelab || workstation && m.status.sshReady
	default:
		return workstation
	}
}

func screenItems(page screen) []menuItem {
	switch page {
	case screenSystem:
		return []menuItem{
			{title: "Mouse personalizado", desc: "Instalar o cursor FrierenBLZ no Niri", action: "cursor"},
			{title: "Migrar pastas para /mnt/sdk", desc: "Mover dados e criar os 23 links aprovados", action: "sdk-links"},
			{title: "Ferramentas Essenciais", desc: "Instalar ferramentas básicas do sistema", goTo: screenEssentials},
			{title: "Ferramentas de Terminal", desc: "Instalar yay e utilitários de linha de comando", goTo: screenTerminalTools},
			{title: "Drivers", desc: "Instalar headers do kernel e utilitários gráficos", goTo: screenDrivers},
			{title: "Fish", desc: "Configurar o shell Fish", goTo: screenFish},
			{title: "Fastfetch", desc: "Configurar as informações exibidas no terminal", goTo: screenFastfetch},
			{title: "Terminal", desc: "Instalar o Kitty", goTo: screenTerminal},
		}
	case screenEssentials:
		return []menuItem{{title: "Instalar ferramentas essenciais", action: "essential-tools"}}
	case screenTerminalTools:
		return []menuItem{
			{title: "Instalar yay", desc: "Compilar o pacote do AUR em ~/.aur/yay", action: "install-yay"},
			{title: "Instalar ferramentas de terminal", desc: "Instalar utilitários como bat, eza e btop", action: "terminal-tools"},
		}
	case screenDrivers:
		return []menuItem{{title: "Instalar headers e utilitários gráficos", action: "drivers"}}
	case screenFish:
		return []menuItem{{title: "Configurar Fish", desc: "Instalar as dependências e aplicar o config.fish", action: "setup-fish"}}
	case screenFastfetch:
		return []menuItem{{title: "Configurar Fastfetch", desc: "Instalar Fastfetch e Chafa e preparar a foto de perfil", action: "setup-fastfetch"}}
	case screenTerminal:
		return []menuItem{{title: "Instalar Kitty", desc: "Instalar o terminal pelo Pacman", action: "install-kitty"}}
	case screenProgramming:
		return []menuItem{
			{title: "Git", desc: "Instalar Git e configurar a assinatura GPG", goTo: screenGit},
			{title: "Mise", desc: "Configurar o Mise e instalar as ferramentas", goTo: screenMise},
		}
	case screenGit:
		return []menuItem{{title: "Configurar Git", desc: "Instalar ferramentas e aplicar a configuração global", action: "setup-git"}}
	case screenMise:
		return []menuItem{{title: "Configurar Mise e instalar ferramentas", desc: "Aplicar o mise.toml e executar mise install", action: "setup-mise"}}
	case screenSecurity:
		return []menuItem{
			{title: "SSH", desc: "Configurar SSH e gerenciar chaves autorizadas", goTo: screenSSH},
			{title: "Firewall", desc: "Ativar o UFW e aplicar a política da LAN", goTo: screenFirewall},
			{title: "Proton Pass", desc: "Instalar a CLI e iniciar o login (requer Firefox configurado)", action: "proton-pass"},
		}
	case screenSSH:
		return []menuItem{
			{title: "Configurar SSH", desc: "Configurar o acesso por chave pública", action: "configure"},
			{title: "Criar chave SSH", desc: "Gerar um par de chaves Ed25519", action: "generate"},
			{title: "Chaves autorizadas", desc: "Adicionar, visualizar e remover chaves públicas", goTo: screenSSHKeys},
		}
	case screenSSHKeys:
		return []menuItem{
			{title: "Adicionar chave pública", desc: "Colar chave pública de um dispositivo", action: "add"},
			{title: "Visualizar chaves atuais", desc: "Mostrar chaves autorizadas e fingerprints", action: "list"},
			{title: "Remover chave pública", desc: "Escolher e confirmar a remoção de uma chave", action: "remove"},
		}
	case screenFirewall:
		return []menuItem{{title: "Configurar Firewall", desc: "Ativar o UFW e aplicar a política da LAN", action: "firewall"}}
	case screenCasaOS:
		return []menuItem{{title: "Instalar CasaOS", desc: "Executar o instalador oficial e preparar o acesso ao Docker", action: "homelab-casaos"}}
	case screenWebBrowser:
		return []menuItem{{title: "Backup Completo", desc: "Salvar todos os perfis e chaves com o Firefox fechado", action: "firefox-backup"}}
	case screenJob:
		return nil
	}
	return []menuItem{
		{title: "Sistema Operacional", desc: "Configurar o sistema e instalar ferramentas", goTo: screenSystem},
		{title: "Segurança", desc: "Configurar SSH, firewall e Proton Pass", goTo: screenSecurity},
		{title: "Navegador Web", desc: "Fazer backup completo do Firefox", goTo: screenWebBrowser},
		{title: "Programação", desc: "Configurar ferramentas de desenvolvimento", goTo: screenProgramming},
		{title: "CasaOS", desc: "Instalar o CasaOS no Ubuntu", goTo: screenCasaOS},
		{title: "Sair", desc: "Até a próxima!", quit: true},
	}
}

func (m model) loadStatus() tea.Cmd {
	return func() tea.Msg {
		return statusResult{m.statusRequest, readNetworkStatus(m.profile)}
	}
}

func (m model) refresh() (model, tea.Cmd) {
	if m.profile.sshKeyName() != "" || m.profile.prefix == "homelab" {
		m.statusRequest++
		return m, m.loadStatus()
	}
	return m, nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.now = time.Time(msg)
		return m, tick()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case statusResult:
		if msg.request == m.statusRequest {
			m.status = msg.status
		}
	case actionResult:
		if m.job != nil {
			m.job.done, m.job.err = true, msg.err
		}
	case tea.KeyMsg:
		if m.screen == screenJob {
			return m.handleJobKey(msg.String())
		}
		return m.handleKey(msg.String())
	}
	return m, nil
}

func (m model) handleJobKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "enter", "esc", "left", "h", "backspace", " ", "q":
		if m.job.done {
			m.job = nil
			return m.back()
		}
	}
	return m, nil
}

func (m model) back() (model, tea.Cmd) {
	if len(m.stack) == 0 {
		return m, nil
	}
	m.screen = m.stack[len(m.stack)-1]
	m.stack = m.stack[:len(m.stack)-1]
	return m.refresh()
}

func (m model) handleKey(key string) (tea.Model, tea.Cmd) {
	items := m.items()
	cur := m.menuCursor(items)
	switch key {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		m.cursor[m.screen] = nextMenuItem(items, cur, -1)
	case "down", "j", "tab":
		m.cursor[m.screen] = nextMenuItem(items, cur, 1)
	case "esc", "left", "h", "backspace":
		return m.back()
	case "enter", "right", "l", " ":
		if len(items) > 0 {
			return m.selectItem(items[cur])
		}
	default:
		if len(key) == 1 {
			if index := int(key[0]) - '1'; index >= 0 && index < len(items) && !items[index].disabled {
				m.cursor[m.screen] = index
				return m.selectItem(items[index])
			}
		}
	}
	return m, nil
}

func (m model) menuCursor(items []menuItem) int {
	index := m.cursor[m.screen]
	if len(items) > 0 && items[index].disabled {
		return nextMenuItem(items, index, 1)
	}
	return index
}

func nextMenuItem(items []menuItem, index, direction int) int {
	for step := 1; step <= len(items); step++ {
		next := (index + step*direction + len(items)) % len(items)
		if !items[next].disabled {
			return next
		}
	}
	return index
}

func (m model) selectItem(item menuItem) (tea.Model, tea.Cmd) {
	switch {
	case item.disabled:
		return m, nil
	case item.quit:
		return m, tea.Quit
	case item.action != "":
		m.stack = append(m.stack, m.screen)
		m.screen = screenJob
		m.job = &jobRun{title: item.title}
		command, err := actionCommand(m.profile.menuAction(item.action), item.title)
		if err != nil {
			m.job.done, m.job.err = true, err
			return m, nil
		}
		return m, tea.ExecProcess(command, func(err error) tea.Msg { return actionResult{err} })
	default:
		m.stack = append(m.stack, m.screen)
		m.screen = item.goTo
		return m.refresh()
	}
}
