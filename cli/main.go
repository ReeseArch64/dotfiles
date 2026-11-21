// Comando dotfiles: configuração interativa por perfil.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const version = "1.1.0"

type profile struct {
	prefix      string
	name        string
	environment string
}

func identifyProfile(hostname string) profile {
	const cachyos = "CachyOS · Niri · Noctalia Shell"
	profiles := map[string]profile{
		"desktop":  {"desktop", "Desktop", cachyos},
		"laptop":   {"laptop", "Laptop", cachyos},
		"homeprod": {"homeprod", "HomeProd", "Ubuntu"},
		"homelab":  {"homelab", "Homelab", "Ubuntu"},
	}
	var detected profile
	for part := range strings.SplitSeq(strings.ToLower(hostname), "-") {
		candidate, ok := profiles[part]
		if !ok {
			continue
		}
		if detected.prefix != "" && detected.prefix != candidate.prefix {
			return profile{}
		}
		detected = candidate
	}
	return detected
}

func (p profile) sshKeyName() string {
	switch p.prefix {
	case "desktop", "laptop":
		return "id_" + p.prefix
	default:
		return ""
	}
}

type tickMsg time.Time
type actionResult struct{ err error }

type model struct {
	profile       profile
	start, now    time.Time
	width, height int
	cursor        int
	screen        string
	running       bool
	result        error
}

func newMenu(hostname string) model {
	now := time.Now()
	return model{profile: identifyProfile(hostname), start: now, now: now, width: 80, height: 24}
}

func tick() tea.Cmd {
	return tea.Tick(time.Second/30, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) Init() tea.Cmd { return tick() }

type menuItem struct {
	title, action string
}

func (m model) items() []menuItem {
	switch m.screen {
	case "SSH":
		return []menuItem{
			{"Configurar SSH", "configure"},
			{"Criar chave SSH", "generate"},
			{"Adicionar chave pública", "add"},
			{"Visualizar chaves autorizadas", "list"},
			{"Remover chave pública", "remove"},
			{"Voltar", "back"},
		}
	case "Firewall":
		return []menuItem{{"Configurar Firewall", "firewall"}, {"Voltar", "back"}}
	}
	if m.profile.sshKeyName() != "" {
		return []menuItem{{"SSH", "SSH"}, {"Firewall", "Firewall"}, {"Sair", "quit"}}
	}
	return []menuItem{{"Sair", "quit"}}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.now = time.Time(msg)
		return m, tick()
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case actionResult:
		m.running, m.result = false, msg.err
	case tea.KeyMsg:
		if m.running {
			return m, nil
		}
		items := m.items()
		key := msg.String()
		switch key {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "esc", "left", "h", "backspace":
			if m.screen != "" {
				return m.selectItem(menuItem{action: "back"})
			}
		case "up", "k":
			m.cursor = (m.cursor - 1 + len(items)) % len(items)
		case "down", "j", "tab":
			m.cursor = (m.cursor + 1) % len(items)
		case "enter", "right", "l", " ":
			return m.selectItem(items[m.cursor])
		default:
			if n, err := strconv.Atoi(key); err == nil && n > 0 && n <= len(items) {
				m.cursor = n - 1
				return m.selectItem(items[m.cursor])
			}
		}
	}
	return m, nil
}

func (m model) selectItem(item menuItem) (tea.Model, tea.Cmd) {
	m.result = nil
	switch item.action {
	case "quit":
		return m, tea.Quit
	case "SSH", "Firewall":
		m.screen, m.cursor = item.action, 0
	case "back":
		m.screen, m.cursor = "", 0
	default:
		command, err := networkCommand(item.action)
		if err != nil {
			m.result = err
			return m, nil
		}
		m.running = true
		return m, tea.ExecProcess(command, func(err error) tea.Msg { return actionResult{err} })
	}
	return m, nil
}

func (m model) subtitle() string {
	if m.profile.prefix == "" {
		return "Sem suporte · by ReeseArch64 · v" + version
	}
	return m.profile.environment + " · by ReeseArch64 · " + m.profile.name + " · v" + version
}

func (m model) View() string {
	width, height := max(m.width, 1), max(m.height, 1)
	elapsed := m.now.Sub(m.start)
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("#6B6B85"))
	subtitle := dim.Width(width).Align(lipgloss.Center).Render(m.subtitle())

	var rows []string
	for i, item := range m.items() {
		title := item.title
		if i == m.cursor {
			rows = append(rows, renderGradientText("▌ "+title, elapsed, true))
		} else {
			rows = append(rows, "  "+title)
		}
	}
	if m.result != nil {
		rows = append(rows, "", lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Render(m.result.Error()))
	}
	title := "Menu principal"
	if m.screen != "" {
		title += " › " + m.screen
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(gradientAt(-elapsed.Seconds()*20).Hex())).
		Padding(1, 2).
		Width(min(max(width-2, 8), 66)).
		Render(dim.Render(title) + "\n\n" + strings.Join(rows, "\n"))
	help := "enter selecionar  ·  q sair"
	if len(m.items()) > 1 {
		help = "↑↓ navegar  ·  " + help
	}
	if m.screen != "" {
		help += "  ·  esc voltar"
	}
	layout := func(compact bool) string {
		return lipgloss.JoinVertical(lipgloss.Center, renderBanner(width, elapsed, compact), "", subtitle, "", box, "", dim.Render(help))
	}
	content := layout(false)
	if lipgloss.Height(content) > height {
		content = layout(true)
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

func main() {
	hostname, _ := os.Hostname()
	if len(os.Args) > 1 && os.Args[1] == "--network" {
		if err := runNetworkAction(identifyProfile(hostname), os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if _, err := tea.NewProgram(newMenu(hostname), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}
