package main

// Execução de ações em passos, mostrados um a um na TUI. Passos comuns rodam
// em background com `sudo -n`; passos interativos (senha do sudo, pacman)
// recebem o terminal via tea.ExecProcess.

import (
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type step struct {
	label string
	run   func() (string, error) // passo em background
	cmd   func() *exec.Cmd       // passo interativo
}

type job struct {
	title  string
	steps  []step
	result func() string // texto exibido após sucesso
}

type stepState int

const (
	stepPending stepState = iota
	stepRunning
	stepOK
	stepFailed
)

type jobRun struct {
	job
	states []stepState
	errOut string
	cur    int
	done   bool
	failed bool
	output string
}

type (
	stepDoneMsg struct {
		out string
		err error
	}
	jobResultMsg string
)

func newJobRun(j job) *jobRun {
	return &jobRun{job: j, states: make([]stepState, len(j.steps))}
}

func (r *jobRun) startStep() tea.Cmd {
	if r.cur >= len(r.steps) {
		r.done = true
		if r.result == nil {
			return nil
		}
		return func() tea.Msg { return jobResultMsg(r.result()) }
	}
	r.states[r.cur] = stepRunning
	s := r.steps[r.cur]
	if s.cmd != nil {
		return tea.ExecProcess(s.cmd(), func(err error) tea.Msg { return stepDoneMsg{"", err} })
	}
	return func() tea.Msg {
		out, err := s.run()
		return stepDoneMsg{out, err}
	}
}

func (r *jobRun) handle(msg stepDoneMsg) tea.Cmd {
	if msg.err != nil {
		r.states[r.cur] = stepFailed
		r.failed, r.done = true, true
		r.errOut = strings.TrimSpace(msg.out)
		if r.errOut == "" {
			r.errOut = msg.err.Error()
		}
		return nil
	}
	r.states[r.cur] = stepOK
	r.cur++
	return r.startStep()
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (r *jobRun) view(width int, elapsed time.Duration) string {
	dim := lipgloss.NewStyle().Foreground(colDim)
	txt := lipgloss.NewStyle().Foreground(colTitle)
	var rows []string
	for i, s := range r.steps {
		var icon, label string
		switch r.states[i] {
		case stepPending:
			icon, label = dim.Render("○"), dim.Render(s.label)
		case stepRunning:
			frame := spinnerFrames[int(elapsed.Milliseconds()/80)%len(spinnerFrames)]
			icon, label = renderGradientText(frame, elapsed, true), renderGradientText(s.label, elapsed, true)
		case stepOK:
			icon, label = lipgloss.NewStyle().Foreground(colOK).Render("✔"), txt.Render(s.label)
		case stepFailed:
			icon, label = lipgloss.NewStyle().Foreground(colErr).Render("✘"), lipgloss.NewStyle().Foreground(colErr).Render(s.label)
		}
		rows = append(rows, icon+" "+label)
	}

	block := lipgloss.NewStyle().Width(width).PaddingLeft(2)
	switch {
	case r.failed:
		rows = append(rows, "", block.Foreground(colErr).Render(lastLines(r.errOut, 8)))
	case r.done:
		rows = append(rows, "", lipgloss.NewStyle().Foreground(colOK).Bold(true).Render("Concluído"))
		if r.output != "" {
			rows = append(rows, "", r.output)
		}
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

// ---- construtores de passos ----

func sudoRun(args ...string) (string, error) {
	out, err := exec.Command("sudo", append([]string{"-n"}, args...)...).CombinedOutput()
	return string(out), err
}

// sudoStep roda um comando como root; soft ignora falhas (ex.: unit inexistente).
func sudoStep(label string, soft bool, args ...string) step {
	return step{label: label, run: func() (string, error) {
		out, err := sudoRun(args...)
		if soft {
			return out, nil
		}
		return out, err
	}}
}

// sudoWrite grava content em path como root (via tee, sem shell).
func sudoWrite(path, content string) (string, error) {
	c := exec.Command("sudo", "-n", "tee", path)
	c.Stdin = strings.NewReader(content)
	out, err := c.CombinedOutput()
	if err != nil {
		return string(out), err
	}
	return "", nil
}

func nativeStep(label string, fn func() error) step {
	return step{label: label, run: func() (string, error) { return "", fn() }}
}

// terminalStep entrega o terminal a um comando, com cabeçalho e pausa em caso de erro.
func terminalStep(label string, argv ...string) step {
	const wrapper = `clear
printf '\n  \e[1;35m━━ %s ━━\e[0m\n\n' "$STEP"
"$@" || { code=$?; read -rsn1 -p $'\n\e[2mPressione qualquer tecla para voltar…\e[0m'; exit $code; }`
	return step{label: label, cmd: func() *exec.Cmd {
		c := exec.Command("bash", append([]string{"-c", wrapper, "dotfiles"}, argv...)...)
		c.Env = append(c.Environ(), "STEP="+label)
		return c
	}}
}

func sudoCached() bool { return exec.Command("sudo", "-n", "true").Run() == nil }

// withSudo prefixa o passo de autenticação quando o sudo ainda pede senha.
func withSudo(steps ...step) []step {
	if sudoCached() {
		return steps
	}
	auth := terminalStep("Autenticar sudo", "sudo", "-v", "-p", "  🔐 Senha de sudo para %u: ")
	return append([]step{auth}, steps...)
}

// ensurePkgs instala via pacman (interativo, para confirmar conflitos) só o que falta.
func ensurePkgs(pkgs ...string) []step {
	missing := missingPkgs(pkgs...)
	if len(missing) == 0 {
		return nil
	}
	return []step{terminalStep("Instalar "+strings.Join(missing, ", "),
		append([]string{"sudo", "pacman", "-S", "--needed"}, missing...)...)}
}

func ensureShellyPkgs(source string, pkgs ...string) []step {
	missing := missingPkgs(pkgs...)
	if len(missing) == 0 {
		return nil
	}
	return []step{terminalStep("Instalar "+strings.Join(missing, ", ")+" via Shelly",
		append([]string{"shelly", "install", source, "--needed"}, missing...)...)}
}
