package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type actionResult struct {
	details string
	err     error
}

type jobTerminalRequest struct {
	command *exec.Cmd
	run     terminalFunc
	reply   chan error
}

// GPG passphrases still use the terminal's no-echo reader, outside the TUI.
type terminalFunc func() error

func (f terminalFunc) Run() error          { return f() }
func (f terminalFunc) SetStdin(io.Reader)  {}
func (f terminalFunc) SetStdout(io.Writer) {}
func (f terminalFunc) SetStderr(io.Writer) {}

type jobTerminalResult struct {
	request jobTerminalRequest
	err     error
}

type jobRun struct {
	title  string
	done   bool
	result actionResult
	events <-chan tea.Msg
	output *jobOutput
}

type jobOutput struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (o *jobOutput) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buffer.Write(p)
}

func (o *jobOutput) text() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.buffer.String()
}

func (r *jobRun) nextEvent() tea.Cmd {
	return func() tea.Msg { return <-r.events }
}

func startLocalJob(p profile, title string, action consoleAction) (*jobRun, tea.Cmd) {
	events := make(chan tea.Msg)
	job := &jobRun{title: title, events: events, output: &jobOutput{}}
	return job, func() tea.Msg {
		if action == nil {
			return actionResult{err: errors.New("ação indisponível")}
		}
		go func() {
			output := job.output
			commands := systemSSHCommands()
			terminal := func(request jobTerminalRequest) error {
				request.reply = make(chan error, 1)
				events <- request
				return <-request.reply
			}
			commands.terminal = func(cmd *exec.Cmd) error { return terminal(jobTerminalRequest{command: cmd}) }
			commands.interactive = func(argv ...string) error {
				if jobCanCapture(argv) {
					text, err := commands.capture("", argv...)
					output.Write([]byte(text))
					return err
				}
				return commands.terminal(exec.Command(argv[0], argv[1:]...))
			}
			console := sshConsole{output: output, commands: commands, withTerminal: func(run func() error) error {
				return terminal(jobTerminalRequest{run: terminalFunc(run)})
			}}
			detailsStart := 0
			console.ask = func(kind promptKind, label string) (string, error) {
				reply := make(chan jobAnswer, 1)
				text := output.text()
				details := text[detailsStart:]
				detailsStart = len(text)
				events <- jobPrompt{kind: kind, label: label, details: details, reply: reply}
				answer := <-reply
				return answer.text, answer.err
			}
			err := action(p, console)
			events <- actionResult{details: output.text(), err: err}
		}()
		return <-events
	}
}

// Keep verified non-interactive tools in the TUI; package managers, sudo and
// external logins retain a terminal instead of swallowing their prompts.
func jobCanCapture(argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	switch filepath.Base(argv[0]) {
	case "curl", "mountpoint":
		return true
	case "env":
		return len(argv) > 3 && argv[1] == "-u" && (argv[2] == "BROWSER" && argv[3] == "xdg-settings" ||
			argv[2] == "BDCLI_DEV_BUILD" && filepath.Base(argv[3]) == "bdcli")
	case "rustup":
		return len(argv) == 3 && argv[1] == "default" && argv[2] == "stable"
	case "git":
		return len(argv) == 5 && argv[1] == "clone" && argv[2] == "--" && argv[3] == yayRepository
	}
	return false
}

func openJobTerminal(request jobTerminalRequest) tea.Cmd {
	result := func(err error) tea.Msg { return jobTerminalResult{request: request, err: err} }
	if request.command != nil {
		return tea.ExecProcess(request.command, result)
	}
	return tea.Exec(request.run, result)
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (r *jobRun) headline(width int, elapsed time.Duration) string {
	status := ""
	if r.done {
		status = "✔ "
		if r.result.err != nil {
			status = "✘ "
		}
	} else {
		status = spinnerFrames[int(elapsed.Milliseconds()/80)%len(spinnerFrames)] + " "
	}
	title := ansi.Truncate(status+r.title, width, "…")
	switch {
	case errors.Is(r.result.err, errActionCanceled):
		return lipgloss.NewStyle().Foreground(colWarn).Render(title)
	case r.result.err != nil:
		return lipgloss.NewStyle().Foreground(colErr).Render(title)
	case r.done:
		return lipgloss.NewStyle().Foreground(colOK).Render(title)
	default:
		return renderGradientText(title, elapsed, true)
	}
}

func (r *jobRun) lines(width int) []string {
	if !r.done {
		if text := r.output.text(); strings.TrimSpace(text) != "" {
			return wrapJobText(text, width)
		}
		return nil
	}
	status := "Concluído"
	if r.result.err != nil {
		status = r.result.err.Error()
	}
	return wrapJobText(status+"\n\n"+r.result.details, width)
}

func wrapJobText(text string, width int) []string {
	text = strings.Map(func(c rune) rune {
		if unicode.IsControl(c) && c != '\n' {
			return -1
		}
		return c
	}, ansi.Strip(strings.ReplaceAll(strings.TrimSpace(text), "\t", "    ")))
	return strings.Split(ansi.Hardwrap(ansi.Wrap(text, width, ""), width, false), "\n")
}

func (m model) jobSize() (width, rows int) {
	return min(max(m.width-4, 1), 66), max(m.height-6, 1)
}

func (m model) viewJob(elapsed time.Duration) string {
	width, rows := m.jobSize()
	lines := m.job.lines(width)
	offset := min(m.jobScroll, max(len(lines)-rows, 0))
	if !m.job.done {
		offset = max(len(lines)-rows, 0)
	}
	parts := []string{m.job.headline(width, elapsed), ""}
	parts = append(parts, lines[offset:min(offset+rows, len(lines))]...)
	help := "Executando…"
	if m.job.done {
		help = "enter/esc voltar"
		if len(lines) > rows {
			help = fmt.Sprintf("↑↓ rolar (%d–%d/%d) · %s", offset+1, offset+rows, len(lines), help)
		}
	}
	return m.viewJobBox(parts, width, help)
}

func (m model) viewJobBox(parts []string, width int, help string) string {
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).
		BorderForeground(colDimmer).Padding(0, 1).Width(width).
		Render(strings.Join(parts, "\n"))
	footer := lipgloss.NewStyle().Foreground(colDim).Render(ansi.Truncate(help, max(m.width, 1), "…"))
	return lipgloss.Place(max(m.width, 1), max(m.height, 1), lipgloss.Center, lipgloss.Center,
		lipgloss.JoinVertical(lipgloss.Center, box, footer))
}
