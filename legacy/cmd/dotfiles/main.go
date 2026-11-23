// Comando dotfiles: menu interativo.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const version = "1.0.3"

type screen int

const (
	screenMain screen = iota
	screenDevelopment
	screenAIAgents
	screenJob
)

var screenTitles = map[screen]string{
	screenMain:        "Menu principal",
	screenDevelopment: "Desenvolvimento",
	screenAIAgents:    "Agentes de IA",
}

type item struct {
	title, desc string
	goTo        screen
	job         func() job
	quit        bool
}

type tickMsg time.Time

type model struct {
	setupDir string
	start    time.Time
	now      time.Time
	width    int
	height   int
	screen   screen
	stack    []screen
	cursor   map[screen]int
	device   deviceProfile
	exitOnly bool
	job      *jobRun
}

var (
	colTitle   = lipgloss.Color("#E4E4F0")
	colDim     = lipgloss.Color("#6B6B85")
	colDimmer  = lipgloss.Color("#45455A")
	colDescSel = lipgloss.Color("#B8B4D8")
	colOK      = lipgloss.Color("#4ADE80")
	colErr     = lipgloss.Color("#F87171")
	colWarn    = lipgloss.Color("#FACC15")
)

func newModel(dotfiles string) model {
	return newModelForDevice(dotfiles, deviceDesktop)
}

func newModelForDevice(dotfiles string, device deviceProfile) model {
	now := time.Now()
	return model{
		setupDir: setupDirectory(dotfiles, device),
		start:    now,
		now:      now,
		width:    80,
		cursor:   map[screen]int{},
		device:   device,
	}
}

func newExitMenu(hostname string) model {
	device := deviceUnknown
	for part := range strings.SplitSeq(strings.ToLower(hostname), "-") {
		if part == "desktop" {
			device = deviceDesktop
			break
		}
	}
	menu := newModelForDevice("", device)
	menu.exitOnly = true
	return menu
}

func (m model) items() []item {
	if m.exitOnly {
		return []item{{title: "Sair", desc: "Até a próxima!", quit: true}}
	}
	if m.device != deviceDesktop {
		return nil
	}
	switch m.screen {
	case screenMain:
		return []item{
			{title: "Desenvolvimento", desc: "Ambientes e ferramentas de desenvolvimento", goTo: screenDevelopment},
			{title: "Agentes de IA", desc: "Configurações de agentes e ferramentas de IA", goTo: screenAIAgents},
			{title: "Sair", desc: "Até a próxima!", quit: true},
		}
	case screenDevelopment:
		return []item{
			{title: "Android CLI", desc: "Instalar a CLI do Android via curl", job: androidCLIJob},
			{title: "Proton VPN CLI", desc: "Instalar pelo yay", job: protonVPNJob},
		}
	case screenAIAgents:
		return []item{{title: "Ollama", desc: "Instalar via curl e configurar serviço para GPU AMD", job: func() job { return ollamaJob(m.setupDir) }}}
	default:
		return nil
	}
}

func tick() tea.Cmd {
	return tea.Tick(time.Second/30, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) Init() tea.Cmd { return tick() }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.now = time.Time(msg)
		return m, tick()

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case stepDoneMsg:
		if m.job != nil {
			return m, m.job.handle(msg)
		}

	case jobResultMsg:
		if m.job != nil {
			m.job.output = string(msg)
		}

	case tea.KeyMsg:
		if m.screen == screenJob {
			return m.handleJobKey(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

// Durante um job só ctrl+c funciona; ao terminar, qualquer tecla de voltar.
func (m model) handleJobKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "enter", "esc", "left", "h", "backspace", " ", "q":
		if m.job.done {
			m.job = nil
			return m.back(), nil
		}
	}
	return m, nil
}

func (m model) back() model {
	if len(m.stack) > 0 {
		m.screen = m.stack[len(m.stack)-1]
		m.stack = m.stack[:len(m.stack)-1]
	}
	return m
}

func (m model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.items()
	cur := m.cursor[m.screen]
	key := k.String()

	switch key {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "up", "k":
		if len(items) > 0 {
			m.cursor[m.screen] = (cur - 1 + len(items)) % len(items)
		}
	case "down", "j", "tab":
		if len(items) > 0 {
			m.cursor[m.screen] = (cur + 1) % len(items)
		}
	case "esc", "left", "h", "backspace":
		return m.back(), nil
	case "enter", "right", "l", " ":
		if cur < len(items) {
			return m.selectItem(items[cur])
		}
	default:
		if len(key) == 1 {
			if n := int(key[0] - '1'); n >= 0 && n < len(items) {
				m.cursor[m.screen] = n
				return m.selectItem(items[n])
			}
		}
	}
	return m, nil
}

func (m model) selectItem(it item) (tea.Model, tea.Cmd) {
	switch {
	case it.quit:
		return m, tea.Quit
	case it.job != nil:
		m.stack = append(m.stack, m.screen)
		m.screen = screenJob
		m.job = newJobRun(it.job())
		return m, m.job.startStep()
	default:
		m.stack = append(m.stack, m.screen)
		m.screen = it.goTo
	}
	return m, nil
}

func (m model) View() string {
	elapsed := m.now.Sub(m.start)
	boxW := min(max(m.width-4, 30), 66)

	inner := boxW - 4
	var body string
	if m.screen == screenJob {
		body = m.job.view(inner, elapsed)
	} else {
		body = m.viewMenu(inner, elapsed)
	}

	crumbs := []string{}
	for _, s := range append(m.stack, m.screen) {
		if s == screenJob {
			crumbs = append(crumbs, m.job.title)
		} else {
			crumbs = append(crumbs, screenTitles[s])
		}
	}
	header := lipgloss.NewStyle().Foreground(colDim).Render(strings.Join(crumbs, " › "))

	border := gradientAt(-elapsed.Seconds() * 20).Hex()
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(border)).
		Padding(1, 2).
		Width(boxW).
		Render(header + "\n\n" + body)

	subtitleStyle := lipgloss.NewStyle().Foreground(colDim)
	if m.exitOnly && m.width > 0 {
		subtitleStyle = subtitleStyle.Width(m.width).Align(lipgloss.Center)
	}
	subtitle := subtitleStyle.Render(m.subtitle())

	help := m.viewHelp(false)
	if lipgloss.Width(help) > m.width {
		help = m.viewHelp(true)
	}

	layout := func(compact bool) string {
		parts := []string{renderBanner(m.width, elapsed, compact), "", subtitle, "", box, "", help}
		return lipgloss.JoinVertical(lipgloss.Center, parts...)
	}
	// Banner grande só quando cabe na altura; senão, versão em uma linha.
	content := layout(false)
	if lipgloss.Height(content) > m.height {
		content = layout(true)
	}

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m model) subtitle() string {
	environment := "CachyOS · Niri · Noctalia Shell"
	switch m.device {
	case deviceUnknown:
		if m.exitOnly {
			environment = "Sem suporte"
		}
	}

	parts := []string{environment}
	if m.exitOnly {
		parts = append(parts, "by ReeseArch64")
	}
	if !m.exitOnly || m.device != deviceUnknown {
		parts = append(parts, m.device.String())
	}
	return strings.Join(append(parts, "v"+version), " · ")
}

func (m model) viewMenu(width int, elapsed time.Duration) string {
	items := m.items()
	if len(items) == 0 {
		return lipgloss.NewStyle().Foreground(colDim).Render("Nenhuma opção disponível.")
	}
	cur := m.cursor[m.screen]
	var rows []string
	for i, it := range items {
		num := fmt.Sprintf("%d", i+1)
		desc := ansi.Truncate(it.desc, width-4, "…")
		if i == cur {
			bar := renderGradientText("▌", elapsed, true)
			title := renderGradientText(it.title, elapsed, true)
			rows = append(rows,
				bar+" "+title,
				bar+" "+lipgloss.NewStyle().Foreground(colDescSel).Render(desc))
		} else {
			rows = append(rows,
				"  "+lipgloss.NewStyle().Foreground(colTitle).Render(it.title)+" "+
					lipgloss.NewStyle().Foreground(colDimmer).Render(num),
				"  "+lipgloss.NewStyle().Foreground(colDim).Render(desc))
		}
		if i < len(items)-1 {
			rows = append(rows, "")
		}
	}
	return strings.Join(rows, "\n")
}

func (m model) viewHelp(compact bool) string {
	key := lipgloss.NewStyle().Foreground(colTitle).Bold(true)
	txt := lipgloss.NewStyle().Foreground(colDim)
	var pairs [][2]string
	if m.screen == screenJob {
		if !m.job.done {
			return txt.Render("executando… ") + key.Render("ctrl+c") + txt.Render(" aborta")
		}
		return key.Render("enter") + " " + txt.Render("voltar")
	}
	if m.exitOnly {
		pairs = append(pairs, [2]string{"enter", "selecionar"})
	} else {
		pairs = append(pairs, [2]string{"↑↓", "navegar"}, [2]string{"enter", "selecionar"}, [2]string{"1-9", "atalho"})
		if len(m.stack) > 0 {
			pairs = append(pairs, [2]string{"esc", "voltar"})
		}
	}
	pairs = append(pairs, [2]string{"q", "sair"})
	var parts []string
	for _, p := range pairs {
		if compact {
			if p[0] != "1-9" {
				parts = append(parts, key.Render(p[0]))
			}
			continue
		}
		parts = append(parts, key.Render(p[0])+" "+txt.Render(p[1]))
	}
	sep := "  ·  "
	if compact {
		sep = " · "
	}
	return strings.Join(parts, txt.Render(sep))
}

// findDotfiles usa $DOTFILES_DIR, depois a pasta acima do binário e por fim ~/.dotfiles.
func findDotfiles() string {
	if d := os.Getenv("DOTFILES_DIR"); d != "" {
		return d
	}
	if exe, err := os.Executable(); err == nil {
		exe, _ = filepath.EvalSymlinks(exe)
		if root := findSetupRepository(filepath.Dir(exe)); root != "" {
			return root
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".dotfiles")
}

func findSetupRepository(directory string) string {
	for {
		if info, err := os.Stat(filepath.Join(directory, "setup")); err == nil && info.IsDir() {
			return directory
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return ""
		}
		directory = parent
	}
}

func main() {
	hostname, _ := os.Hostname()
	p := tea.NewProgram(newExitMenu(hostname), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}
