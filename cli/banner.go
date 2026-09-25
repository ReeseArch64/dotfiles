package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/lucasb-eyer/go-colorful"
)

// Fonte "ANSI Shadow": só os glifos usados no banner.
var glyphs = map[rune][]string{
	'A': {" █████╗ ", "██╔══██╗", "███████║", "██╔══██║", "██║  ██║", "╚═╝  ╚═╝"},
	'C': {" ██████╗", "██╔════╝", "██║     ", "██║     ", "╚██████╗", " ╚═════╝"},
	'D': {"██████╗ ", "██╔══██╗", "██║  ██║", "██║  ██║", "██████╔╝", "╚═════╝ "},
	'E': {"███████╗", "██╔════╝", "█████╗  ", "██╔══╝  ", "███████╗", "╚══════╝"},
	'F': {"███████╗", "██╔════╝", "█████╗  ", "██╔══╝  ", "██║     ", "╚═╝     "},
	'H': {"██╗  ██╗", "██║  ██║", "███████║", "██╔══██║", "██║  ██║", "╚═╝  ╚═╝"},
	'I': {"██╗", "██║", "██║", "██║", "██║", "╚═╝"},
	'L': {"██╗     ", "██║     ", "██║     ", "██║     ", "███████╗", "╚══════╝"},
	'O': {" ██████╗ ", "██╔═══██╗", "██║   ██║", "██║   ██║", "╚██████╔╝", " ╚═════╝ "},
	'R': {"██████╗ ", "██╔══██╗", "██████╔╝", "██╔══██╗", "██║  ██║", "╚═╝  ╚═╝"},
	'S': {"███████╗", "██╔════╝", "███████╗", "╚════██║", "███████║", "╚══════╝"},
	'T': {"████████╗", "╚══██╔══╝", "   ██║   ", "   ██║   ", "   ██║   ", "   ╚═╝   "},
	'6': {" ██████╗ ", "██╔════╝ ", "███████╗ ", "██╔═══██╗", "╚██████╔╝", " ╚═════╝ "},
	'4': {"██╗  ██╗", "██║  ██║", "███████║", "╚════██║", "     ██║", "     ╚═╝"},
}

const glyphHeight = 6

// Paleta cíclica no estilo do Copilot CLI: violeta → magenta → rosa → laranja → ciano → índigo.
var paletteStops = []string{"#8B5CF6", "#D946EF", "#F43F5E", "#FB923C", "#FACC15", "#22D3EE", "#6366F1"}

const lutSize = 512

var lut = buildLUT()

func buildLUT() []colorful.Color {
	stops := make([]colorful.Color, len(paletteStops))
	for i, hex := range paletteStops {
		stops[i], _ = colorful.Hex(hex)
	}
	out := make([]colorful.Color, lutSize)
	seg := float64(lutSize) / float64(len(stops))
	for i := range out {
		pos := float64(i) / seg
		a := stops[int(pos)%len(stops)]
		b := stops[(int(pos)+1)%len(stops)]
		out[i] = a.BlendLab(b, pos-math.Floor(pos)).Clamped()
	}
	return out
}

// gradientAt devolve a cor da paleta numa posição contínua (em "colunas").
func gradientAt(pos float64) colorful.Color {
	i := int(math.Floor(pos*4)) % lutSize
	if i < 0 {
		i += lutSize
	}
	return lut[i]
}

func fg(c colorful.Color) string {
	r, g, b := c.RGB255()
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
}

const reset = "\x1b[0m"

func renderWord(word string) []string {
	rows := make([]string, glyphHeight)
	for _, r := range word {
		g := glyphs[r]
		w := 0
		for _, row := range g {
			w = max(w, len([]rune(row)))
		}
		for i := range rows {
			row := g[i]
			rows[i] += row + strings.Repeat(" ", w-len([]rune(row)))
		}
	}
	return rows
}

// bannerLines escolhe o layout que cabe na largura do terminal.
func bannerLines(width int) []string {
	layouts := [][]string{
		{"REESEARCH64", "DOTFILES"},
		{"REESE", "ARCH64", "DOTFILES"},
	}
	for _, words := range layouts {
		var lines []string
		fits := true
		for i, w := range words {
			rows := renderWord(w)
			if len([]rune(rows[0])) > width-2 {
				fits = false
				break
			}
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, rows...)
		}
		if fits {
			return lines
		}
	}
	return nil
}

func isShadow(r rune) bool { return r != '█' && r != ' ' }

// renderBanner pinta o banner com um gradiente diagonal que flui com o tempo,
// uma faixa de brilho que varre periodicamente e uma revelação inicial.
func renderBanner(width int, elapsed time.Duration, compact bool) string {
	t := elapsed.Seconds()
	var lines []string
	if !compact {
		lines = bannerLines(width)
	}
	if lines == nil {
		return renderGradientText("ReeseArch64 Dotfiles", elapsed, true)
	}

	maxW := 0
	for _, l := range lines {
		maxW = max(maxW, len([]rune(l)))
	}

	const revealDur = 0.9
	reveal := math.Inf(1)
	if t < revealDur {
		p := t / revealDur
		reveal = (1 - math.Pow(1-p, 3)) * float64(maxW+len(lines)*2)
	}

	shimmerPeriod := 3.5
	shimmer := math.Mod(t, shimmerPeriod)/shimmerPeriod*float64(maxW+60) - 30
	white := colorful.Color{R: 1, G: 1, B: 1}
	black := colorful.Color{R: 0.05, G: 0.03, B: 0.1}

	var b strings.Builder
	for y, line := range lines {
		runes := []rune(line)
		pad := (maxW - len(runes)) / 2
		b.WriteString(strings.Repeat(" ", pad))
		for i, r := range runes {
			x := float64(i + pad)
			diag := x + float64(y)*2
			if r == ' ' || diag > reveal {
				b.WriteRune(' ')
				continue
			}
			c := gradientAt(x*0.9 + float64(y)*1.6 - t*28)
			if d := math.Abs(x - float64(y)*0.6 - shimmer); d < 6 {
				c = c.BlendLab(white, (1-d/6)*0.65).Clamped()
			}
			if reveal-diag < 3 {
				c = white
			}
			if isShadow(r) {
				c = c.BlendRgb(black, 0.55)
			}
			b.WriteString(fg(c))
			b.WriteRune(r)
		}
		b.WriteString(reset)
		if y < len(lines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// renderGradientText pinta um texto curto com o mesmo gradiente animado.
func renderGradientText(s string, elapsed time.Duration, bold bool) string {
	t := elapsed.Seconds()
	var b strings.Builder
	if bold {
		b.WriteString("\x1b[1m")
	}
	for i, r := range []rune(s) {
		if r == ' ' {
			b.WriteRune(r)
			continue
		}
		b.WriteString(fg(gradientAt(float64(i)*2.2 - t*28)))
		b.WriteRune(r)
	}
	b.WriteString(reset)
	return b.String()
}
