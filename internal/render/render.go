// Package render turns Widget payloads into styled text for a given size and
// draws the tile chrome around them.
package render

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/oronbz/ghud/internal/layout"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

type Tile struct {
	Title   string
	W, H    int
	Focused bool
	// Arrange draws the heavy warn border of the Widget being arranged.
	Arrange bool
	// Handle draws the ⠿ grab handle before the title.
	Handle  bool
	Payload widget.Payload
	// List is how a list Payload is selected and scrolled.
	List List
	// Hints are the key hints drawn in the bottom border.
	Hints []Hint
	// Status is drawn at the right of the top border.
	Status Status
}

// Status is how a tile's top border decorates its Widget's Status: the
// spinner while refreshing, the stale marker, or the age of cached data.
type Status struct {
	// Spinner is the spinner frame while the Widget refreshes, or "".
	Spinner string
	// Stale is the stale marker, reason and age such as "offline · 12m", or "".
	Stale string
	// Age is how old data read from the cache is, such as "3m", drawn dim
	// while it isn't stale.
	Age string
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
	b := lipgloss.RoundedBorder()
	if t.Focused || t.Arrange {
		border = th.Fg(theme.Accent)
		title = th.Fg(theme.Bright).Bold(true)
		count = th.Fg(theme.Accent)
	}
	if t.Arrange {
		border = th.Fg(theme.Warn)
		b = lipgloss.ThickBorder()
	}
	inner := t.W - 2

	label := " " + title.Render(t.Title) + " "
	if t.Handle {
		label = " " + th.Fg(theme.Accent).Render(layout.HandleGlyph) + title.Render(t.Title) + " "
	}
	if t.Payload.IsList() {
		label += count.Render(fmt.Sprint(t.Payload.Count)) + " "
	}
	right := status(th, t.Status, t.W-5-lipgloss.Width(label))
	fill := t.W - 3 - lipgloss.Width(label) - lipgloss.Width(right)
	if right != "" {
		fill--
		right += border.Render(b.Top)
	}
	top := border.Render(b.TopLeft+b.Top) + label + border.Render(strings.Repeat(b.Top, max(0, fill))) + right + border.Render(b.TopRight)

	out := make([]string, 0, t.H)
	out = append(out, ansi.Truncate(top, t.W, ""))
	t.List.Focused = t.Focused
	for _, l := range body(th, t.Payload, t.List, inner, t.H-2) {
		out = append(out, border.Render(b.Left)+l+border.Render(b.Right))
	}
	out = append(out, bottom(th, border, b, t.Hints, t.W))
	return strings.Join(out, "\n")
}

// status is the stale marker in warn, or else the age in dim, and the
// spinner in accent, in at most room cells; the marker is cut short to fit,
// and left out when it can't.
func status(th theme.Theme, st Status, room int) string {
	spin := ""
	if st.Spinner != "" {
		spin = st.Spinner + " "
	}
	marker, tok := st.Stale, theme.Warn
	if marker == "" {
		marker, tok = st.Age, theme.Dim
	}
	stale := ""
	if marker != "" {
		if w := room - lipgloss.Width(spin) - 2; w >= min(4, lipgloss.Width(marker)) {
			stale = th.Fg(tok).Render(ansi.Truncate(marker, w, "…")) + " "
		}
	}
	if stale == "" && (spin == "" || room < 3) {
		return ""
	}
	return " " + stale + th.Fg(theme.Accent).Render(spin)
}

// bottom is the bottom border of a tile w wide, carrying " key label "
// hints while they fit.
func bottom(th theme.Theme, border lipgloss.Style, edge lipgloss.Border, hints []Hint, w int) string {
	var b strings.Builder
	x := 2
	b.WriteString(border.Render(edge.BottomLeft + edge.Bottom))
	for i, at := range HintSlots(hints, w) {
		b.WriteString(" " + th.Fg(theme.Accent).Bold(true).Render(KeyLabel(hints[i].Key)) + th.Fg(theme.Dim).Render(" "+hints[i].Label) + " ")
		b.WriteString(border.Render(edge.Bottom))
		x = at.X + at.W + 1
	}
	b.WriteString(border.Render(strings.Repeat(edge.Bottom, max(0, w-1-x)) + edge.BottomRight))
	return b.String()
}

// Slot is where a hint sits in the bottom border, in cells from the tile's left edge.
type Slot struct{ X, W int }

// HintSlots places the hints that fit in the bottom border of a tile w wide,
// in order; the rest are left out.
func HintSlots(hints []Hint, w int) []Slot {
	var out []Slot
	x := 2
	for _, h := range hints {
		hw := ansi.StringWidth(" " + KeyLabel(h.Key) + " " + h.Label + " ")
		if x+hw+1 > w-2 {
			break
		}
		out = append(out, Slot{X: x, W: hw})
		x += hw + 1
	}
	return out
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
	if p.Wrap {
		return textBody(th, p, l, w, h)
	}
	lines := liveLines(th, p.Lines, w, h)

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

func styled(th theme.Theme, spans []widget.Span, w int) string {
	var b strings.Builder
	bars := barWidths(spans, w)
	for i, s := range spans {
		if s.Bar != nil {
			b.WriteString(bar(th, s, bars[i]))
			continue
		}
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
