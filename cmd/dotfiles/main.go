// Comando dotfiles: menu interativo para configurar esta máquina.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const version = "0.14.0"

type screen int

const (
	screenMain screen = iota
	screenSystem
	screenDevelopment
	screenDesktop
	screenAIAgents
	screenBackup
	screenPiAgent
	screenSSH
	screenFirewall
	screenDocker
	screenGit
	screenGPG
	screenNoctalia
	screenCleanup
	screenJob
)

var screenTitles = map[screen]string{
	screenMain:        "Menu principal",
	screenSystem:      "Sistema",
	screenDevelopment: "Desenvolvimento",
	screenDesktop:     "Desktop",
	screenAIAgents:    "Agentes de IA",
	screenBackup:      "Backup",
	screenPiAgent:     "Pi Agent",
	screenSSH:         "SSH",
	screenFirewall:    "Firewall",
	screenDocker:      "Docker",
	screenGit:         "Git",
	screenGPG:         "GPG",
	screenNoctalia:    "Noctalia",
	screenCleanup:     "Aplicativos pré-instalados",
}

type item struct {
	title, desc string
	goTo        screen
	job         func() job
	quit        bool
}

type tickMsg time.Time

type model struct {
	dotfiles string
	start    time.Time
	now      time.Time
	width    int
	height   int
	screen   screen
	stack    []screen
	cursor   map[screen]int
	ssh      sshStatus
	fw       fwStatus
	docker   dockerStatus
	git      gitStatus
	gpg      gpgStatus
	noctalia noctaliaStatus
	piAgent  piAgentStatus
	job      *jobRun
}

var (
	colTitle   = lipgloss.Color("#E4E4F0")
	colDim     = lipgloss.Color("#6B6B85")
	colDimmer  = lipgloss.Color("#45455A")
	colDescSel = lipgloss.Color("#B8B4D8")
	colOK      = lipgloss.Color("#4ADE80")
	colErr     = lipgloss.Color("#F87171")
)

func newModel(dotfiles string) model {
	now := time.Now()
	return model{
		dotfiles: dotfiles,
		start:    now,
		now:      now,
		width:    80,
		cursor:   map[screen]int{},
	}
}

func (m model) items() []item {
	switch m.screen {
	case screenMain:
		return []item{
			{title: "Sistema", desc: "SSH, firewall e Docker", goTo: screenSystem},
			{title: "Desenvolvimento", desc: "Git, ambientes e IDEs", goTo: screenDevelopment},
			{title: "Desktop", desc: "Aplicativos, aparência e ambiente gráfico", goTo: screenDesktop},
			{title: "Agentes de IA", desc: "Configurações de agentes e ferramentas de IA", goTo: screenAIAgents},
			{title: "Backup", desc: "Salvar dados pessoais em /mnt/backups", goTo: screenBackup},
			{title: "Sair", desc: "Até a próxima!", quit: true},
		}
	case screenSystem:
		return []item{
			{title: "SSH", desc: "Configurar cliente, servidor permanente e acesso pela LAN", goTo: screenSSH},
			{title: "Firewall", desc: "Ativar o UFW e liberar o SSH somente na LAN", goTo: screenFirewall},
			{title: "Docker", desc: "Instalar ferramentas, ativar o serviço e autenticar", goTo: screenDocker},
			{title: "Drivers", desc: "Instalar base-devel, Vulkan, glxinfo e headers do Linux", job: driversJob},
			{title: "Aplicativos pré-instalados", desc: "Remover aplicativos do CachyOS após instalar o Ghostty", goTo: screenCleanup},
		}
	case screenDevelopment:
		return []item{
			{title: "Git", desc: "Instalar ferramentas e configurar os arquivos globais", goTo: screenGit},
			{title: "GPG", desc: "Instalar GnuPG e importar chaves após configurar o Git", goTo: screenGPG},
			{title: "Ambiente de Desenvolvimento", desc: "Configurar mise, linguagens, Rust e autenticação npm", job: func() job { return developmentEnvironmentJob(m.dotfiles) }},
			{title: "Ambiente Sandbox", desc: "Configurar Podman e criar um Distrobox Arch Linux isolado", job: sandboxEnvironmentJob},
			{title: "Instalar IDEs", desc: "Instalar Visual Studio Code e Zed via Shelly", job: idesJob},
			{title: "Ferramentas de terminal", desc: "Instalar editores e utilitários de terminal", job: func() job { return terminalToolsJob(m.dotfiles) }},
		}
	case screenDesktop:
		return []item{
			{title: "Niri", desc: "Copiar configuração e cursor FrierenBLZ", job: func() job { return niriJob(m.dotfiles) }},
			{title: "Noctalia", desc: "Verificar plugins e configurar ~/.local/state/noctalia", goTo: screenNoctalia},
			{title: "Ghostty", desc: "Instalar o terminal e copiar sua configuração", job: func() job { return ghosttyJob(m.dotfiles) }},
			{title: "Zen Browser", desc: "Instalar o navegador e restaurar o backup local", job: func() job { return zenBrowserJob(m.dotfiles) }},
			{title: "Thunderbird", desc: "Instalar o cliente de e-mail via Pacman", job: thunderbirdJob},
			{title: "Discord", desc: "Instalar Discord, BetterDiscord e aplicar sua configuração", job: func() job { return discordJob(m.dotfiles) }},
			{title: "Obsidian", desc: "Instalar o aplicativo e copiar sua configuração", job: func() job { return obsidianJob(m.dotfiles) }},
			{title: "Wallpapers", desc: "Copiar imagens para ~/.wallpapers", job: func() job { return wallpapersJob(m.dotfiles) }},
			{title: "Foto de perfil", desc: "Criar ~/.face usando a imagem deste repositório", job: func() job { return faceJob(m.dotfiles) }},
		}
	case screenAIAgents:
		return []item{
			{title: "Pi Agent", desc: "Tema, settings e pacotes do Pi", goTo: screenPiAgent},
		}
	case screenBackup:
		return []item{
			{title: "Backup do Navegador", desc: "Salvar Zen em /mnt/backups/zen-backup.tar", job: browserBackupJob},
		}
	case screenPiAgent:
		return m.piAgentItems()
	case screenSSH:
		return m.sshItems()
	case screenFirewall:
		return m.fwItems()
	case screenDocker:
		return m.dockerItems()
	case screenGit:
		return m.gitItems()
	case screenGPG:
		return m.gpgItems()
	case screenNoctalia:
		return m.noctaliaItems()
	case screenCleanup:
		return m.cleanupItems()
	}
	return nil
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
		m.refresh()
	}
	return m
}

// refresh relê o estado usado pela tela atual.
func (m *model) refresh() {
	switch m.screen {
	case screenSSH:
		m.ssh = loadSSHStatus()
	case screenFirewall:
		m.fw = loadFWStatus(loadSSHStatus().port())
	case screenDocker:
		m.docker = loadDockerStatus()
	case screenGit:
		m.git = loadGitStatus(m.dotfiles)
	case screenGPG:
		m.gpg = loadGPGStatus(m.dotfiles)
	case screenNoctalia:
		m.noctalia = loadNoctaliaStatus(m.dotfiles)
	case screenPiAgent:
		m.piAgent = loadPiAgentStatus(m.dotfiles)
	}
}

func (m model) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.items()
	cur := m.cursor[m.screen]

	switch k.String() {
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
		if n := int(k.String()[0] - '1'); len(k.String()) == 1 && n >= 0 && n < len(items) {
			m.cursor[m.screen] = n
			return m.selectItem(items[n])
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
		m.refresh()
	}
	return m, nil
}

func (m model) View() string {
	elapsed := m.now.Sub(m.start)
	boxW := min(max(m.width-4, 30), 66)

	inner := boxW - 4
	divider := lipgloss.NewStyle().Foreground(colDimmer).Render(strings.Repeat("─", inner))
	var body string
	switch m.screen {
	case screenJob:
		body = m.job.view(inner, elapsed)
	case screenSSH:
		body = m.ssh.card(inner) + "\n\n" + divider + "\n\n" + m.viewMenu(inner, elapsed)
	case screenFirewall:
		body = m.fw.card(inner) + "\n\n" + divider + "\n\n" + m.viewMenu(inner, elapsed)
	case screenDocker:
		body = m.docker.card(inner) + "\n\n" + divider + "\n\n" + m.viewMenu(inner, elapsed)
	case screenGit:
		body = m.git.card(inner) + "\n\n" + divider + "\n\n" + m.viewMenu(inner, elapsed)
	case screenGPG:
		body = m.gpg.card(inner) + "\n\n" + divider + "\n\n" + m.viewMenu(inner, elapsed)
	case screenNoctalia:
		body = m.noctalia.card(inner) + "\n\n" + divider + "\n\n" + m.viewMenu(inner, elapsed)
	case screenPiAgent:
		body = m.piAgent.card(inner) + "\n\n" + divider + "\n\n" + m.viewMenu(inner, elapsed)
	default:
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

	subtitle := lipgloss.NewStyle().Foreground(colDim).Render("CachyOS · Niri · Noctalia Shell · v" + version)

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
	pairs = append(pairs, [2]string{"↑↓", "navegar"}, [2]string{"enter", "selecionar"}, [2]string{"1-9", "atalho"})
	if len(m.stack) > 0 {
		pairs = append(pairs, [2]string{"esc", "voltar"})
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
		for d := filepath.Dir(exe); d != "/"; d = filepath.Dir(d) {
			if _, err := os.Stat(filepath.Join(d, "configs")); err == nil {
				return d
			}
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".dotfiles")
}

func main() {
	dotfiles := findDotfiles()
	if err := checkPlatform("/etc/os-release", exec.LookPath); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
	p := tea.NewProgram(newModel(dotfiles), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		os.Exit(1)
	}
}
