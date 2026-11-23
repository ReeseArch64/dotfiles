package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

var (
	colTitle   = lipgloss.Color("#E4E4F0")
	colDim     = lipgloss.Color("#6B6B85")
	colDimmer  = lipgloss.Color("#45455A")
	colDescSel = lipgloss.Color("#B8B4D8")
	colOK      = lipgloss.Color("#4ADE80")
	colErr     = lipgloss.Color("#F87171")
	colWarn    = lipgloss.Color("#FACC15")
)

func (m model) subtitle() string {
	if m.profile.prefix == "" {
		return "Sem suporte · by ReeseArch64 · v" + version
	}
	return m.profile.environment + " · by ReeseArch64 · " + m.profile.name + " · v" + version
}

func screenTitle(page screen) string {
	if login, ok := miseLoginForScreen(page); ok {
		return login.title
	}
	return screenTitles[page]
}

func (m model) View() string {
	width, height := max(m.width, 1), max(m.height, 1)
	elapsed := m.now.Sub(m.start)
	boxW := min(max(width-4, 30), 66)
	inner := boxW - 4

	var crumbs []string
	for _, s := range m.stack {
		crumbs = append(crumbs, screenTitle(s))
	}
	switch m.screen {
	case screenJob:
		crumbs = append(crumbs, m.job.title)
	default:
		crumbs = append(crumbs, screenTitle(m.screen))
	}
	dim := lipgloss.NewStyle().Foreground(colDim)
	breadcrumb := strings.Join(crumbs, " › ")
	if len(crumbs) > 1 && lipgloss.Width(breadcrumb) > inner {
		last := len(crumbs) - 1
		breadcrumb = strings.Join(crumbs[:last], " › ") + " ›\n" + crumbs[last]
	}
	header := dim.Render(breadcrumb)
	subtitle := dim.Width(width).Align(lipgloss.Center).Render(m.subtitle())
	help := m.viewHelp(false)
	if lipgloss.Width(help) > width {
		help = m.viewHelp(true)
	}
	menuLimit := 0
	layout := func(compactBanner, compactMenu, selectedDescriptionOnly bool) string {
		var body string
		switch m.screen {
		case screenJob:
			body = m.job.view(inner, elapsed)
		case screenSSH, screenFirewall, screenDocker:
			divider := lipgloss.NewStyle().Foreground(colDimmer).Render(strings.Repeat("─", inner))
			card := m.status.card(m.screen, inner, compactMenu)
			if m.screen == screenDocker {
				card = renderCard(m.docker.rows(), inner)
			}
			gap := "\n\n"
			if compactMenu {
				gap = "\n"
			}
			body = card + gap + divider + gap + m.viewMenu(inner, elapsed, compactMenu, selectedDescriptionOnly, menuLimit)
		default:
			body = m.viewMenu(inner, elapsed, compactMenu, selectedDescriptionOnly, menuLimit)
		}
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(gradientAt(-elapsed.Seconds()*20).Hex())).
			Padding(1, 2).
			Width(boxW).
			Render(header + "\n\n" + body)
		return lipgloss.JoinVertical(lipgloss.Center, renderBanner(width, elapsed, compactBanner), "", subtitle, "", box, "", help)
	}
	content := layout(false, false, false)
	if lipgloss.Height(content) > height {
		content = layout(true, false, false)
	}
	if lipgloss.Height(content) > height {
		content = layout(true, true, false)
	}
	if lipgloss.Height(content) > height {
		content = layout(true, true, true)
	}
	if (m.screen == screenMiseLogins || m.screen == screenSystem) && lipgloss.Height(content) > height {
		menuLimit = max(1, len(m.items())-(lipgloss.Height(content)-height))
		content = layout(true, true, true)
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

func (m model) viewMenu(width int, elapsed time.Duration, compact, selectedDescriptionOnly bool, limit int) string {
	items := m.items()
	if len(items) == 0 {
		return lipgloss.NewStyle().Foreground(colDim).Render("Nenhuma opção disponível.")
	}
	cur := m.menuCursor(items)
	start, end := 0, len(items)
	if limit > 0 && limit < end {
		start = min(max(cur-limit/2, 0), end-limit)
		end = start + limit
	}
	var rows []string
	for i := start; i < end; i++ {
		it := items[i]
		var shortcut string
		if i < 9 {
			shortcut = fmt.Sprintf(" %d", i+1)
		}
		desc := ansi.Truncate(it.desc, width-4, "…")
		var title, description string
		if it.disabled {
			dim := lipgloss.NewStyle().Foreground(colDimmer)
			title = "  " + dim.Render(it.title+shortcut)
			description = "  " + dim.Render(desc)
		} else if i == cur {
			bar := renderGradientText("▌", elapsed, true)
			title = bar + " " + renderGradientText(it.title, elapsed, true)
			description = bar + " " + lipgloss.NewStyle().Foreground(colDescSel).Render(desc)
		} else {
			title = "  " + lipgloss.NewStyle().Foreground(colTitle).Render(it.title) +
				lipgloss.NewStyle().Foreground(colDimmer).Render(shortcut)
			description = "  " + lipgloss.NewStyle().Foreground(colDim).Render(desc)
		}
		rows = append(rows, title)
		if !selectedDescriptionOnly || i == cur {
			rows = append(rows, description)
		}
		if !compact && i < end-1 {
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
