package main

// Execução de ações em passos, mostrados um a um na TUI. Passos comuns rodam
// em background com `sudo -n`; passos interativos (senha do sudo, pacman)
// recebem o terminal via tea.ExecProcess.

import (
	"context"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type step struct {
	label    string
	run      func() (string, error) // passo em background
	cmd      func() *exec.Cmd       // passo interativo
	skip     func() bool
	optional bool
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
	stepSkipped
	stepIgnored
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
	s := r.steps[r.cur]
	if s.skip != nil && s.skip() {
		r.states[r.cur] = stepSkipped
		r.cur++
		return r.startStep()
	}
	r.states[r.cur] = stepRunning
	if s.cmd != nil {
		return tea.ExecProcess(s.cmd(), func(err error) tea.Msg { return stepDoneMsg{"", err} })
	}
	return func() tea.Msg {
		out, err := s.run()
		return stepDoneMsg{out, err}
	}
}

func (r *jobRun) handle(msg stepDoneMsg) tea.Cmd {
	if msg.err != nil && r.steps[r.cur].optional {
		r.states[r.cur] = stepIgnored
		r.cur++
		return r.startStep()
	}
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
		case stepSkipped:
			icon, label = dim.Render("−"), dim.Render(s.label+" (já configurado)")
		case stepIgnored:
			icon, label = lipgloss.NewStyle().Foreground(colWarn).Render("!"), dim.Render(s.label+" (falhou, ignorado)")
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

func nativeStep(label string, fn func() error) step {
	return step{label: label, run: func() (string, error) { return "", fn() }}
}

// terminalStep entrega o terminal a um comando, com cabeçalho e pausa em caso de erro.
func terminalStep(label string, argv ...string) step {
	return terminalStepWithPause(label, true, argv...)
}

func optionalTerminalStep(label string, argv ...string) step {
	current := terminalStepWithPause(label, false, argv...)
	current.optional = true
	return current
}

func terminalStepWithPause(label string, pause bool, argv ...string) step {
	failure := `read -rsn1 -p $'\n\e[2mPressione qualquer tecla para voltar…\e[0m'`
	if !pause {
		failure = `printf '\nLogin opcional não concluído; continuando a configuração.\n'`
	}
	wrapper := `clear
printf '\n  \e[1;35m━━ %s ━━\e[0m\n\n' "$STEP"
"$@" || { code=$?; ` + failure + `; exit $code; }`
	return step{label: label, cmd: func() *exec.Cmd {
		c := exec.Command("bash", append([]string{"-c", wrapper, "dotfiles"}, argv...)...)
		c.Env = append(c.Environ(), "STEP="+label)
		return c
	}}
}

func skipWhen(current step, check func() bool) step {
	current.skip = check
	return current
}

func commandSucceeds(env []string, name string, args ...string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	if env != nil {
		cmd.Env = env
	}
	return cmd.Run() == nil
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

func pacmanSyncStep() step {
	return terminalStep("Atualizar bases do Pacman", "sudo", "pacman", "-Syy")
}

func pacmanInstallStep(pkgs ...string) step {
	return terminalStep("Instalar "+strings.Join(pkgs, ", "),
		append([]string{"sudo", "pacman", "-S", "--needed"}, pkgs...)...)
}

func ensurePkgs(pkgs ...string) []step {
	missing := missingPkgs(pkgs...)
	if len(missing) == 0 {
		return nil
	}
	return []step{pacmanSyncStep(), pacmanInstallStep(missing...)}
}

const shellyFallbackScript = `source=$1
package=$2
if shelly install "$source" "$package" && pacman -Q "$package"; then
    exit 0
fi
aur_root="$HOME/.aur"
mkdir -p "$aur_root"
cd "$aur_root" || exit 1
if [ ! -d "$package/.git" ]; then
    if [ -e "$package" ]; then
        printf 'Diretório AUR inválido: %s\n' "$aur_root/$package" >&2
        exit 1
    fi
    git clone "https://aur.archlinux.org/$package.git" || exit 1
fi
cd "$package" || exit 1
makepkg -si || exit 1
pacman -Q "$package"`

func shellyInstallStep(source, pkg string) step {
	return terminalStep("Instalar "+pkg+" via Shelly",
		"bash", "-c", shellyFallbackScript, "dotfiles-shelly-fallback", source, pkg)
}

func ensureShellyPkgs(source string, pkgs ...string) []step {
	missing := missingPkgs(pkgs...)
	if len(missing) == 0 {
		return nil
	}
	var steps []step
	for _, pkg := range missing {
		steps = append(steps, pacmanSyncStep(), shellyInstallStep(source, pkg))
	}
	return steps
}
