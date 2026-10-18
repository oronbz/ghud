// Package render turns Widget payloads into styled text for a given size and
// draws the tile chrome around them.
package render

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

type Tile struct {
	Title   string
	W, H    int
	Focused bool
	Payload widget.Payload
}

// Draw renders a rounded-border tile exactly W×H cells, title in the top border.
func Draw(th theme.Theme, t Tile) string {
	if t.W < 2 || t.H < 2 {
		return ""
	}
	border := th.Fg(theme.Line)
	title := th.Fg(theme.Fg)
	if t.Focused {
		border = th.Fg(theme.Accent)
		title = th.Fg(theme.Bright).Bold(true)
	}
	inner := t.W - 2

	label := " " + title.Render(t.Title) + " "
	top := border.Render("╭─") + label + border.Render(strings.Repeat("─", max(0, t.W-3-lipgloss.Width(label)))) + border.Render("╮")

	out := make([]string, 0, t.H)
	out = append(out, ansi.Truncate(top, t.W, ""))
	for _, l := range Body(th, t.Payload, inner, t.H-2) {
		out = append(out, border.Render("│")+l+border.Render("│"))
	}
	out = append(out, border.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return strings.Join(out, "\n")
}

// Body renders a payload as exactly h lines, each exactly w cells wide.
func Body(th theme.Theme, p widget.Payload, w, h int) []string {
	var lines []string
	for _, l := range p.Lines {
		if l.Big {
			lines = append(lines, bigLines(th, l)...)
			continue
		}
		lines = append(lines, styled(th, l.Spans))
	}

	top := 0
	if p.Center {
		top = max(0, (h-len(lines))/2)
	}
	out := make([]string, h)
	for i := range out {
		s := ""
		if j := i - top; j >= 0 && j < len(lines) {
			s = lines[j]
		}
		out[i] = fit(s, w, p.Center)
	}
	return out
}

func styled(th theme.Theme, spans []widget.Span) string {
	var b strings.Builder
	for _, s := range spans {
		st := th.Fg(s.Token)
		if s.Bold {
			st = st.Bold(true)
		}
		b.WriteString(st.Render(s.Text))
	}
	return b.String()
}

func fit(s string, w int, center bool) string {
	s = ansi.Truncate(s, w, "…")
	gap := w - lipgloss.Width(s)
	left := 0
	if center {
		left = gap / 2
	}
	return strings.Repeat(" ", left) + s + strings.Repeat(" ", gap-left)
}
