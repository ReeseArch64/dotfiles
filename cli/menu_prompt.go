package main

import (
	"fmt"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type jobAnswer struct {
	text string
	err  error
}

type jobPrompt struct {
	kind           promptKind
	label, details string
	reply          chan jobAnswer
	input          string
	confirm        bool
	scroll         int
}

func (m model) handlePromptKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	p := *m.prompt
	answer := jobAnswer{}
	submit := false
	switch key.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		answer.err, submit = errActionCanceled, true
	case tea.KeyEnter:
		answer.text = strings.TrimSpace(p.input)
		submit = p.kind != promptText || answer.text != ""
		if p.kind == promptConfirm && !p.confirm {
			answer.err = errActionCanceled
		}
	case tea.KeyLeft, tea.KeyRight, tea.KeyTab, tea.KeyShiftTab:
		if p.kind == promptConfirm {
			p.confirm = !p.confirm
		}
	case tea.KeyUp, tea.KeyPgUp, tea.KeyDown, tea.KeyPgDown:
		lines, rows := m.promptDetails()
		step := 1
		if key.Type == tea.KeyPgUp || key.Type == tea.KeyPgDown {
			step = rows
		}
		if key.Type == tea.KeyUp || key.Type == tea.KeyPgUp {
			step = -step
		}
		limit := max(len(lines)-rows, 0)
		p.scroll = min(max(min(p.scroll, limit)+step, 0), limit)
	case tea.KeyBackspace:
		if p.kind == promptText && p.input != "" {
			runes := []rune(p.input)
			p.input = string(runes[:len(runes)-1])
		}
	case tea.KeyCtrlU:
		p.input = ""
	case tea.KeyRunes, tea.KeySpace:
		if p.kind == promptText {
			// A multiline paste must never silently become a single public key.
			if strings.IndexFunc(string(key.Runes), unicode.IsControl) < 0 && len(p.input)+len(string(key.Runes)) <= 16384 {
				p.input += string(key.Runes)
			}
		}
	}
	if submit {
		p.reply <- answer
		m.prompt = nil
		return m, m.job.nextEvent()
	}
	m.prompt = &p
	return m, nil
}

func (m model) promptDetails() ([]string, int) {
	width, rows := m.jobSize()
	if strings.TrimSpace(m.prompt.details) == "" {
		return nil, 1
	}
	return wrapJobText(m.prompt.details, width), max(1, rows-len(wrapJobText(m.prompt.label, width))-2)
}

func (m model) viewPrompt() string {
	p := m.prompt
	width, _ := m.jobSize()
	parts := []string{ansi.Truncate(m.job.title, width, "…")}
	lines, rows := m.promptDetails()
	help := "enter continuar · esc cancelar"
	if len(lines) > 0 {
		offset := min(p.scroll, max(len(lines)-rows, 0))
		parts = append(parts, lines[offset:min(offset+rows, len(lines))]...)
		if len(lines) > rows {
			help = fmt.Sprintf("↑↓ %d–%d/%d · %s", offset+1, offset+rows, len(lines), help)
		}
	}
	parts = append(parts, "", strings.Join(wrapJobText(p.label, width), "\n"))
	switch p.kind {
	case promptConfirm:
		choices := "[ Cancelar ]    Confirmar"
		if p.confirm {
			choices = "  Cancelar    [ Confirmar ]"
		}
		parts = append(parts, ansi.Truncate(choices, width, "…"))
		help = strings.Replace(help, "enter continuar", "←→ escolher · enter selecionar", 1)
	case promptContinue:
		parts = append(parts, ansi.Truncate("[ Continuar ]", width, "…"))
	case promptText:
		input := ansi.TruncateLeft(p.input, max(width-3, 0), "…")
		parts = append(parts, ansi.Truncate("> "+input+"▏", width, "…"))
	}
	return m.viewJobBox(parts, width, help)
}
