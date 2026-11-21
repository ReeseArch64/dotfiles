package main

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

type jobRun struct {
	title string
	done  bool
	err   error
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (r *jobRun) view(width int, elapsed time.Duration) string {
	txt := lipgloss.NewStyle().Foreground(colTitle)
	var rows []string
	switch {
	case r.err != nil:
		red := lipgloss.NewStyle().Foreground(colErr)
		rows = append(rows, red.Render("✘ "+r.title))
	case r.done:
		rows = append(rows, lipgloss.NewStyle().Foreground(colOK).Render("✔")+" "+txt.Render(r.title))
	default:
		frame := spinnerFrames[int(elapsed.Milliseconds()/80)%len(spinnerFrames)]
		rows = append(rows, renderGradientText(frame, elapsed, true)+" "+renderGradientText(r.title, elapsed, true))
	}

	block := lipgloss.NewStyle().Width(width).PaddingLeft(2)
	switch {
	case r.err != nil:
		rows = append(rows, "", block.Foreground(colErr).Render(lastLines(r.err.Error(), 8)))
	case r.done:
		rows = append(rows, "", lipgloss.NewStyle().Foreground(colOK).Bold(true).Render("Concluído"))
	}
	return strings.Join(rows, "\n")
}

func lastLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
