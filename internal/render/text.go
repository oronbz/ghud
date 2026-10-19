package render

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/oronbz/visor/internal/theme"
	"github.com/oronbz/visor/internal/widget"
)

// TextRows is how many lines a wrapped Payload takes in a body w cells
// wide; the host clamps its scroll offset with it.
func TextRows(p widget.Payload, w int) int { return len(wrapped(theme.Dark(), p, w)) }

// wrapped is a wrapped Payload's lines, word-wrapped inside a one-cell margin each side.
func wrapped(th theme.Theme, p widget.Payload, w int) []string {
	cw := max(1, w-2)
	var out []string
	for _, l := range p.Lines {
		out = append(out, strings.Split(lipgloss.Wrap(styled(th, l.Spans, cw), cw, ""), "\n")...)
	}
	return out
}

// textBody draws a wrapped Payload as exactly h lines of w cells from line
// l.Top, with a scroll thumb in the right margin when it overflows.
func textBody(th theme.Theme, p widget.Payload, l List, w, h int) []string {
	lines := wrapped(th, p, w)
	cw := max(0, w-2)
	thumbAt, thumbLen := -1, 0
	if len(lines) > h && h > 0 {
		thumbLen = max(1, h*h/len(lines))
		thumbAt = (h - thumbLen) * l.Top / (len(lines) - h)
	}
	out := make([]string, h)
	for i := range out {
		s := ""
		if j := l.Top + i; j >= 0 && j < len(lines) {
			s = lines[j]
		}
		right := " "
		if i >= thumbAt && i < thumbAt+thumbLen {
			right = th.Fg(theme.Faint).Render("▐")
		}
		out[i] = " " + fit(s, cw, false) + right
	}
	return out
}
