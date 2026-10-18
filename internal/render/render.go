// Package render turns Widget payloads into styled text for a given size and
// draws the tile chrome around them.
package render

import (
	"fmt"
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
	// List is how a list Payload is selected and scrolled.
	List List
	// Hints are the key hints drawn in the bottom border.
	Hints []Hint
}

// Hint is one key hint: the key as Bubble Tea prints it, and what it does.
type Hint struct{ Key, Label string }

// Draw renders a rounded-border tile exactly W×H cells, title in the top border.
func Draw(th theme.Theme, t Tile) string {
	if t.W < 2 || t.H < 2 {
		return ""
	}
	border := th.Fg(theme.Line)
	title := th.Fg(theme.Fg)
	count := th.Fg(theme.Dim)
	if t.Focused {
		border = th.Fg(theme.Accent)
		title = th.Fg(theme.Bright).Bold(true)
		count = th.Fg(theme.Accent)
	}
	inner := t.W - 2

	label := " " + title.Render(t.Title) + " "
	if t.Payload.IsList() {
		label += count.Render(fmt.Sprint(t.Payload.Count)) + " "
	}
	top := border.Render("╭─") + label + border.Render(strings.Repeat("─", max(0, t.W-3-lipgloss.Width(label)))) + border.Render("╮")

	out := make([]string, 0, t.H)
	out = append(out, ansi.Truncate(top, t.W, ""))
	t.List.Focused = t.Focused
	for _, l := range body(th, t.Payload, t.List, inner, t.H-2) {
		out = append(out, border.Render("│")+l+border.Render("│"))
	}
	out = append(out, bottom(th, border, t.Hints, inner))
	return strings.Join(out, "\n")
}

// bottom is the bottom border, carrying " key label " hints while they fit.
func bottom(th theme.Theme, border lipgloss.Style, hints []Hint, inner int) string {
	var b strings.Builder
	used := 1
	b.WriteString(border.Render("╰─"))
	for _, h := range hints {
		text := " " + th.Fg(theme.Accent).Bold(true).Render(KeyLabel(h.Key)) + th.Fg(theme.Dim).Render(" "+h.Label) + " "
		w := lipgloss.Width(text) + 1
		if used+w > inner-1 {
			break
		}
		b.WriteString(text + border.Render("─"))
		used += w
	}
	b.WriteString(border.Render(strings.Repeat("─", max(0, inner-used)) + "╯"))
	return b.String()
}

// KeyLabel is how a key is shown in hints: enter and space as glyphs.
func KeyLabel(key string) string {
	switch key {
	case "enter":
		return "⏎"
	case "space":
		return "␣"
	}
	return key
}

// body renders a payload as exactly h lines, each exactly w cells wide.
func body(th theme.Theme, p widget.Payload, l List, w, h int) []string {
	if p.IsList() {
		return listBody(th, p, l, w, h)
	}
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
		if s.Bg != theme.Bg {
			st = st.Background(th.Color(s.Bg))
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
