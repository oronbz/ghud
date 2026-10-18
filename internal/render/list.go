package render

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

// Lines in Rows that draw no Item; every such mark is negative.
const (
	Header = -1
	Ruled  = -2
)

// Rows is a list's rendered line list: for each line, the index of the Item
// it draws, or a negative mark (Header, Ruled). Selection, scrolling and
// clicks all map through it.
func Rows(p widget.Payload) []int {
	var rows []int
	group := ""
	for i, it := range p.Items {
		if p.Rule != nil && p.Rule.Before == i {
			rows = append(rows, Ruled)
		}
		if it.Group != group && it.Group != "" {
			rows = append(rows, Header)
		}
		group = it.Group
		rows = append(rows, i)
	}
	if p.Rule != nil && p.Rule.Before >= len(p.Items) {
		rows = append(rows, Ruled)
	}
	return rows
}

// List is how a list is drawn: which Item is selected and the first line shown.
type List struct {
	Selected, Top int
	// Focused highlights the selected Item.
	Focused bool
}

// minFlex is the narrowest a flex column gets before other columns drop.
const minFlex = 10

// listBody draws exactly h lines of w cells: a one-cell margin each side,
// and a scroll thumb in the right margin when the list overflows.
func listBody(th theme.Theme, p widget.Payload, l List, w, h int) []string {
	out := make([]string, h)
	cw := max(0, w-2)
	if len(p.Items) == 0 {
		for i := range out {
			out[i] = strings.Repeat(" ", w)
		}
		if h > 0 {
			out[0] = " " + fit(th.Fg(theme.Faint).Render(p.Empty), cw, false) + " "
		}
		return out
	}
	rows := Rows(p)
	widths := columnWidths(p, cw)
	thumbAt, thumbLen := -1, 0
	if len(rows) > h && h > 0 {
		thumbLen = max(1, h*h/len(rows))
		thumbAt = (h - thumbLen) * l.Top / (len(rows) - h)
	}
	for i := range out {
		line := strings.Repeat(" ", cw)
		if j := l.Top + i; j >= 0 && j < len(rows) {
			switch rows[j] {
			case Header:
				line = header(th, p.Items[rows[j+1]].Group, cw)
			case Ruled:
				line = rule(th, *p.Rule, cw)
			default:
				line = row(th, p, rows[j], widths, cw, l.Focused && rows[j] == l.Selected)
			}
		}
		right := " "
		if i >= thumbAt && i < thumbAt+thumbLen {
			right = th.Fg(theme.Faint).Render("▐")
		}
		out[i] = " " + line + right
	}
	return out
}

func header(th theme.Theme, group string, w int) string {
	text := ansi.Truncate(group, w, "…")
	rule := max(0, w-ansi.StringWidth(text)-1)
	if rule == 0 {
		return fit(th.Fg(theme.Dim).Bold(true).Render(text), w, false)
	}
	return th.Fg(theme.Dim).Bold(true).Render(text) + " " + th.Fg(theme.Line).Render(strings.Repeat("─", rule))
}

func rule(th theme.Theme, r widget.Rule, w int) string {
	line := strings.Repeat("─", w)
	if r.Text != "" {
		line = r.Text + " " + line
	}
	return th.Fg(r.Token).Render(ansi.Truncate(line, w, ""))
}

func row(th theme.Theme, p widget.Payload, item int, widths []int, w int, selected bool) string {
	bg := lipgloss.NewStyle()
	if selected {
		bg = bg.Background(th.Color(theme.Sel))
	}
	var b strings.Builder
	used := 0
	for c, cw := range widths {
		if cw < 0 {
			continue
		}
		if used > 0 {
			b.WriteString(bg.Render(" "))
			used++
		}
		var f widget.Field
		if fs := p.Items[item].Fields; c < len(fs) {
			f = fs[c]
		}
		text := ansi.Truncate(fieldText(th, f, bg, selected), cw, "…")
		gap := bg.Render(strings.Repeat(" ", cw-ansi.StringWidth(text)))
		if p.Columns[c].Right {
			b.WriteString(gap + text)
		} else {
			b.WriteString(text + gap)
		}
		used += cw
	}
	b.WriteString(bg.Render(strings.Repeat(" ", max(0, w-used))))
	return ansi.Truncate(b.String(), w, "")
}

// fieldText styles a field; on the selected row, plain text turns bright and bold.
func fieldText(th theme.Theme, f widget.Field, bg lipgloss.Style, selected bool) string {
	var b strings.Builder
	for _, s := range f {
		st := bg.Foreground(th.Color(s.Token)).Bold(s.Bold)
		if selected && s.Token == theme.Fg {
			st = st.Foreground(th.Color(theme.Bright)).Bold(true)
		}
		if s.Bg != theme.Bg {
			st = st.Background(th.Color(s.Bg))
		}
		b.WriteString(st.Render(s.Text))
	}
	return b.String()
}

// columnWidths gives each column its width in w cells, or -1 once it is
// dropped. Columns drop highest Priority first until the rest fit, one cell
// apart, with every flex column at least minFlex wide (or its widest Field).
func columnWidths(p widget.Payload, w int) []int {
	natural := make([]int, len(p.Columns))
	for _, it := range p.Items {
		for c := range p.Columns {
			if c < len(it.Fields) {
				natural[c] = max(natural[c], fieldWidth(it.Fields[c]))
			}
		}
	}
	shown := make([]bool, len(p.Columns))
	for c := range shown {
		shown[c] = true
	}
	need := func() (fixed, flexes int, total int) {
		n := 0
		for c, col := range p.Columns {
			if !shown[c] {
				continue
			}
			n++
			if col.Flex {
				flexes++
				total += min(natural[c], minFlex)
			} else {
				fixed += natural[c]
				total += natural[c]
			}
		}
		return fixed, flexes, total + max(0, n-1)
	}
	for {
		_, _, total := need()
		if total <= w {
			break
		}
		drop := -1
		for c, col := range p.Columns {
			if shown[c] && (drop < 0 || col.Priority > p.Columns[drop].Priority) {
				drop = c
			}
		}
		if drop < 0 || countTrue(shown) == 1 {
			break
		}
		shown[drop] = false
	}
	fixed, flexes, _ := need()
	widths := make([]int, len(p.Columns))
	gaps := max(0, countTrue(shown)-1)
	spare := max(0, w-fixed-gaps)
	for c, col := range p.Columns {
		switch {
		case !shown[c]:
			widths[c] = -1
		case col.Flex:
			widths[c] = spare / flexes
		default:
			widths[c] = natural[c]
		}
	}
	return widths
}

func fieldWidth(f widget.Field) int {
	n := 0
	for _, s := range f {
		n += ansi.StringWidth(s.Text)
	}
	return n
}

func countTrue(bs []bool) int {
	n := 0
	for _, b := range bs {
		if b {
			n++
		}
	}
	return n
}
