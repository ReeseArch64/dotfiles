package main

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type screen int

const (
	screenMain screen = iota
	screenSystem
	screenSSH
	screenSSHKeys
	screenFirewall
	screenWebBrowser
	screenJob
)

var screenTitles = map[screen]string{
	screenMain:       "Menu principal",
	screenSystem:     "Sistema Operacional",
	screenSSH:        "SSH",
	screenSSHKeys:    "Chaves autorizadas",
	screenFirewall:   "Firewall",
	screenWebBrowser: "Navegador Web",
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
	if m.profile.sshKeyName() == "" {
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
	switch m.screen {
	case screenSystem:
		firewallDescription := "Ativar UFW e permitir SSH somente na LAN"
		if !m.status.sshReady {
			firewallDescription = "Requer SSH configurado"
		}
		return []menuItem{
			{title: "SSH", desc: "Configurar SSH e gerenciar chaves autorizadas", goTo: screenSSH},
			{title: "Firewall", desc: firewallDescription, goTo: screenFirewall, disabled: !m.status.sshReady},
			{title: "Mouse personalizado", desc: "Instalar o cursor FrierenBLZ no Niri", action: "cursor"},
		}
	case screenSSH:
		return []menuItem{
			{title: "Configurar SSH", desc: "Configurar cliente e permitir acesso somente por chave", action: "configure"},
			{title: "Criar chave SSH", desc: "Gerar o par Ed25519 " + m.profile.sshKeyName(), action: "generate"},
			{title: "Chaves autorizadas", desc: "Adicionar, visualizar e remover chaves públicas", goTo: screenSSHKeys},
		}
	case screenSSHKeys:
		return []menuItem{
			{title: "Adicionar chave pública", desc: "Colar chave pública de um dispositivo", action: "add"},
			{title: "Visualizar chaves atuais", desc: "Mostrar chaves autorizadas e fingerprints", action: "list"},
			{title: "Remover chave pública", desc: "Escolher e confirmar a remoção de uma chave", action: "remove"},
		}
	case screenFirewall:
		return []menuItem{{title: "Configurar Firewall", desc: "Ativar UFW e permitir SSH somente na LAN", action: "firewall", disabled: !m.status.sshReady}}
	case screenWebBrowser:
		return []menuItem{{title: "Backup Completo", desc: "Salvar todos os perfis e chaves com o Firefox fechado", action: "firefox-backup"}}
	case screenJob:
		return nil
	}
	quit := menuItem{title: "Sair", desc: "Até a próxima!", quit: true}
	if m.profile.sshKeyName() == "" {
		return []menuItem{quit}
	}
	return []menuItem{
		{title: "Sistema Operacional", desc: "Configurar SSH, firewall e cursor", goTo: screenSystem},
		{title: "Navegador Web", desc: "Fazer backup completo do Firefox", goTo: screenWebBrowser},
		quit,
	}
}

func (m model) loadStatus() tea.Cmd {
	return func() tea.Msg {
		return statusResult{m.statusRequest, readNetworkStatus(m.profile)}
	}
}

func (m model) refresh() (model, tea.Cmd) {
	if m.profile.sshKeyName() != "" {
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
	cur := m.cursor[m.screen]
	switch key {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		m.cursor[m.screen] = (cur - 1 + len(items)) % len(items)
	case "down", "j", "tab":
		m.cursor[m.screen] = (cur + 1) % len(items)
	case "esc", "left", "h", "backspace":
		return m.back()
	case "enter", "right", "l", " ":
		return m.selectItem(items[cur])
	default:
		if len(key) == 1 {
			if index := int(key[0]) - '1'; index >= 0 && index < len(items) {
				m.cursor[m.screen] = index
				return m.selectItem(items[index])
			}
		}
	}
	return m, nil
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
		command, err := actionCommand(item.action, item.title)
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
