package render

import (
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/oronbz/visor/internal/theme"
	"github.com/oronbz/visor/internal/widget"
)

// liveLines expands a live view's lines and host helpers into rows w cells
// wide at most, giving charts without a height the rows left of h.
func liveLines(th theme.Theme, lines []widget.Line, w, h int) []string {
	used, fills := 0, 0
	for _, l := range lines {
		switch {
		case l.Chart != nil && l.Chart.Rows == 0:
			fills++
		case l.Chart != nil:
			used += l.Chart.Rows
		case l.Digits != widget.NoDigits:
			used += DigitsSize("", l.Digits).H
		default:
			used++
		}
	}
	spare := max(0, h-used)
	var out []string
	for _, l := range lines {
		switch {
		case l.Chart != nil:
			rows := l.Chart.Rows
			if rows == 0 {
				rows = spare / fills
				spare -= rows
				fills--
			}
			out = append(out, chart(th, *l.Chart, w, rows)...)
		case l.Digits != widget.NoDigits:
			out = append(out, digitLines(th, l)...)
		default:
			out = append(out, styled(th, l.Spans, w))
		}
	}
	return out
}

// braille dot bits by dot row, top first, for a cell's left and right columns.
var (
	leftDots  = [4]rune{0x01, 0x02, 0x04, 0x40}
	rightDots = [4]rune{0x08, 0x10, 0x20, 0x80}
)

// chart draws the newest values that fit as a filled braille area, two
// samples per cell, rows cells tall.
func chart(th theme.Theme, c widget.Chart, w, rows int) []string {
	if rows <= 0 || w <= 0 {
		return nil
	}
	values := c.Values[max(0, len(c.Values)-2*w):]
	// The first sample goes in a right column when the count is odd, so the newest always ends the last cell.
	offset := 2*w - len(values)
	cells := make([][]rune, rows)
	for r := range cells {
		cells[r] = make([]rune, w)
	}
	for i, v := range values {
		slot := offset + i
		col, dots := slot/2, leftDots
		if slot%2 == 1 {
			dots = rightDots
		}
		level := int(math.Round(min(max(v, 0), 1) * float64(rows*4)))
		for d := range level {
			row := rows - 1 - d/4
			cells[row][col] |= dots[3-d%4]
		}
	}
	st := th.Fg(c.Token)
	out := make([]string, rows)
	for r, row := range cells {
		var b strings.Builder
		for _, bits := range row {
			if bits == 0 {
				b.WriteByte(' ')
			} else {
				b.WriteRune(0x2800 + bits)
			}
		}
		out[r] = st.Render(b.String())
	}
	return out
}

// barWidths gives each bar span its width: its own, or a share of what the
// line's other spans leave of w.
func barWidths(spans []widget.Span, w int) []int {
	widths := make([]int, len(spans))
	left, fills := w, 0
	for i, s := range spans {
		switch {
		case s.Bar == nil:
			left -= ansi.StringWidth(s.Text)
		case s.Bar.Width > 0:
			widths[i] = s.Bar.Width
			left -= s.Bar.Width
		default:
			fills++
		}
	}
	left = max(0, left)
	for i, s := range spans {
		if s.Bar != nil && s.Bar.Width == 0 {
			widths[i] = left / fills
			left -= widths[i]
			fills--
		}
	}
	return widths
}

var eighths = []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// bar fills Value of w cells in eighth blocks, over a track in the line token.
func bar(th theme.Theme, s widget.Span, w int) string {
	if w <= 0 {
		return ""
	}
	n := int(math.Round(min(max(s.Bar.Value, 0), 1) * float64(w*8)))
	filled := strings.Repeat("█", n/8) + eighths[n%8]
	cells := n/8 + min(1, n%8)
	return th.Fg(s.Token).Render(filled) + th.Fg(theme.Line).Render(strings.Repeat("─", w-cells))
}
