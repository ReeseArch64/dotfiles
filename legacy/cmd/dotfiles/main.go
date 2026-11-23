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
	screenSSHKeys
	screenWebBrowser
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
	screenSSHKeys:     "Chaves autorizadas",
	screenWebBrowser:  "Navegador Web",
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
	disabled    bool
}

type tickMsg time.Time

type model struct {
	dotfiles string
	setupDir string
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
)

func newModel(dotfiles string) model {
	return newModelForDevice(dotfiles, deviceDesktop)
}

func newModelForDevice(dotfiles string, device deviceProfile) model {
	now := time.Now()
	return model{
		dotfiles: dotfiles,
		setupDir: setupDirectory(dotfiles, device),
		start:    now,
		now:      now,
		width:    80,
		cursor:   map[screen]int{},
		device:   device,
	}
}

func newExitMenu(hostname string) model {
	profiles := map[string]deviceProfile{
		"desktop":  deviceDesktop,
		"laptop":   deviceLaptop,
		"homeprod": deviceHomeProd,
		"homelab":  deviceServer,
	}
	device := deviceUnknown
	for part := range strings.SplitSeq(strings.ToLower(hostname), "-") {
		profile := profiles[part]
		if profile == deviceUnknown {
			continue
		}
		if device != deviceUnknown && device != profile {
			device = deviceUnknown
			break
		}
		device = profile
	}
	menu := newModelForDevice("", device)
	menu.exitOnly = true
	return menu
}

func (m model) items() []item {
	if m.exitOnly {
		return []item{{title: "Sair", desc: "Até a próxima!", quit: true}}
	}
	if m.device == deviceLaptop {
		return m.laptopItems()
	}
	switch m.device {
	case deviceDesktop, deviceNotebook, deviceServer, deviceHomeProd:
	default:
		return nil
	}
	if m.device == deviceServer {
		return m.serverItems()
	}
	if m.device == deviceHomeProd {
		return m.homeProdItems()
	}
	switch m.screen {
	case screenMain:
		systemDescription := "SSH, firewall e Docker"
		developmentDescription := "Git, ambientes e IDEs"
		if m.device == deviceDesktop {
			systemDescription = "SSH e firewall"
			developmentDescription = "Ambientes e ferramentas de desenvolvimento"
		}
		return []item{
			{title: "Sistema", desc: systemDescription, goTo: screenSystem},
			{title: "Desenvolvimento", desc: developmentDescription, goTo: screenDevelopment},
			{title: "Desktop", desc: "Aplicativos, aparência e ambiente gráfico", goTo: screenDesktop},
			{title: "Agentes de IA", desc: "Configurações de agentes e ferramentas de IA", goTo: screenAIAgents},
			{title: "Backup", desc: "Salvar dados pessoais em /mnt/backups/dotfiles", goTo: screenBackup},
			{title: "Sair", desc: "Até a próxima!", quit: true},
		}
	case screenSystem:
		sshDescription := "Configurar cliente e servidor permanente com acesso pela LAN"
		firewallDescription := "Ativar o UFW e liberar o SSH somente na LAN"
		if m.device == deviceDesktop {
			sshDescription = "Configurar somente o cliente e bloquear conexões recebidas"
			firewallDescription = "Ativar o UFW sem liberar SSH de entrada"
		}
		items := []item{
			{title: "SSH", desc: sshDescription, goTo: screenSSH},
			{title: "Firewall", desc: firewallDescription, goTo: screenFirewall},
		}
		if m.device == deviceNotebook {
			items = append(items,
				item{title: "Docker", desc: "Instalar ferramentas, ativar o serviço e autenticar", goTo: screenDocker},
				item{title: "Aplicativos pré-instalados", desc: "Remover aplicativos do CachyOS após instalar o Kitty", goTo: screenCleanup},
			)
		}
		return items
	case screenDevelopment:
		var items []item
		if m.device == deviceNotebook {
			items = append(items,
				item{title: "Git", desc: "Instalar ferramentas e configurar os arquivos globais", goTo: screenGit},
				item{title: "GPG", desc: "Gerar chave GPG por dispositivo após configurar o Git", goTo: screenGPG},
				item{title: "Ambiente de Desenvolvimento", desc: "Configurar mise, linguagens, Rust e autenticação npm", job: func() job { return developmentEnvironmentJob(m.setupDir) }},
			)
		}
		if m.device == deviceDesktop {
			items = append(items,
				item{title: "Ambiente Sandbox", desc: "Configurar Podman e criar um Distrobox Arch Linux isolado", job: sandboxEnvironmentJob},
				item{title: "Instalar Zed", desc: "Instalar o editor Zed via yay", job: zedJob},
			)
		}
		if m.device == deviceNotebook {
			items = append(items, item{title: "Linear CLI", desc: "Instalar a CLI do Linear via curl", job: linearCLIJob})
		}
		items = append(items, item{title: "Android CLI", desc: "Instalar a CLI do Android via curl", job: androidCLIJob})
		if m.device == deviceDesktop {
			return append(items, item{title: "Proton VPN CLI", desc: "Instalar pelo yay", job: protonVPNJob})
		}
		return append(items, item{title: "Ferramentas de terminal", desc: "Instalar editores e utilitários de terminal", job: func() job { return terminalToolsJob(m.setupDir) }})
	case screenDesktop:
		var items []item
		if m.device == deviceNotebook {
			items = append(items,
				item{title: "Niri", desc: "Copiar configuração e cursor FrierenBLZ", job: func() job { return niriJob(m.setupDir) }},
				item{title: "Login automático", desc: "Entrar direto no Niri ao ligar, sem senha no SDDM", job: autologinJob},
			)
		}
		if m.device == deviceDesktop {
			items = append(items,
				item{title: "Discord", desc: "Instalar Discord, BetterDiscord e aplicar sua configuração", job: func() job { return discordJob(m.setupDir) }},
				item{title: "Obsidian", desc: "Instalar o aplicativo e copiar sua configuração", job: func() job { return obsidianJob(m.setupDir) }},
				item{title: "Turing Smart Screen", desc: "Instalar com Python 3.13, GPU AMD e tema Frieren", job: func() job { return turingScreenJob(m.setupDir) }},
			)
		}
		if m.device == deviceNotebook {
			items = append(items,
				item{title: "Noctalia", desc: "Verificar plugins e configurar ~/.local/state/noctalia", goTo: screenNoctalia},
				item{title: "Mozilla Firefox", desc: "Instalar o navegador e defini-lo como padrão", job: firefoxJob},
				item{title: "Wallpapers", desc: "Copiar imagens para ~/.wallpapers", job: func() job { return wallpapersJob(m.dotfiles) }},
			)
		}
		return append(items, item{title: "Foto de perfil", desc: "Criar ~/.face usando a imagem deste repositório", job: func() job { return faceJob(m.dotfiles) }})
	case screenAIAgents:
		if m.device == deviceDesktop {
			return []item{{title: "Ollama", desc: "Instalar via curl e configurar serviço para GPU AMD", job: func() job { return ollamaJob(m.setupDir) }}}
		}
		return []item{{title: "Pi Agent", desc: "Tema, settings e pacotes do Pi", goTo: screenPiAgent}}
	case screenBackup:
		return []item{
			{title: "Ponto de Restauração", desc: "Criar snapshot Btrfs da raiz com data e hora", job: restorePointJob},
			{title: "Backup de Projetos Locais", desc: "Enviar projetos ao GitHub, GitLab e Gitea", job: func() job { return projectsBackupJob(m.device) }},
			{title: "Backup das Chaves SSH", desc: "Copiar chaves deste dispositivo para /mnt/backups/dotfiles/ssh", job: func() job { return sshKeysBackupJob(m.device) }},
			{title: "Backup das Chaves GPG", desc: "Exportar chaves pública e privada para /mnt/backups/dotfiles/gpg", job: gpgBackupJob},
		}
	case screenPiAgent:
		if m.device == deviceNotebook {
			return m.piAgentItems()
		}
	case screenSSH:
		return m.sshItems()
	case screenFirewall:
		return m.fwItems()
	case screenDocker:
		if m.device == deviceNotebook {
			return m.dockerItems()
		}
	case screenGit:
		if m.device == deviceNotebook {
			return m.gitItems()
		}
	case screenGPG:
		if m.device == deviceNotebook {
			return m.gpgItems()
		}
	case screenNoctalia:
		if m.device == deviceNotebook {
			return m.noctaliaItems()
		}
	case screenCleanup:
		if m.device == deviceNotebook {
			return m.cleanupItems()
		}
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
	if m.device == deviceLaptop {
		m.ssh = loadSSHStatusForDevice(deviceLaptop)
	}
	switch m.screen {
	case screenSSH:
		m.ssh = loadSSHStatusForDevice(m.device)
	case screenFirewall:
		m.fw = loadFWStatusForDevice(m.device, loadSSHStatusForDevice(m.device).port())
	case screenDocker:
		m.docker = loadDockerStatus()
	case screenGit:
		m.git = loadGitStatus(m.setupDir)
	case screenGPG:
		m.gpg = loadGPGStatus(m.setupDir)
	case screenNoctalia:
		m.noctalia = loadNoctaliaStatus(m.setupDir)
	case screenPiAgent:
		m.piAgent = loadPiAgentStatus(m.setupDir)
	}
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
	if it.disabled {
		return m, nil
	}
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
	case deviceLaptop:
		if !m.exitOnly {
			environment = "Omarchy · Wayland"
		}
	case deviceServer, deviceHomeProd:
		environment = "Ubuntu"
	case deviceUnknown:
		if m.exitOnly {
			environment = "Sem suporte"
		}
	}

	parts := []string{environment}
	if m.exitOnly || m.device == deviceLaptop {
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
		if it.disabled {
			dim := lipgloss.NewStyle().Foreground(colDimmer)
			rows = append(rows,
				"  "+dim.Render(it.title+" "+num),
				"  "+dim.Render(desc))
		} else if i == cur {
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
