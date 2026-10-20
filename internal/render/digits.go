package render

import (
	"strings"
	"unicode"

	"github.com/oronbz/visor/internal/theme"
	"github.com/oronbz/visor/internal/widget"
)

// glyphs are 5-dot-tall bitmaps, one string per dot row. Half-block digits
// pack two dot rows into a cell; block digits draw each dot two cells wide.
var glyphs = map[rune][5]string{
	'0': {"111", "101", "101", "101", "111"},
	'1': {"011", "001", "001", "001", "001"},
	'2': {"111", "001", "111", "100", "111"},
	'3': {"111", "001", "011", "001", "111"},
	'4': {"101", "101", "111", "001", "001"},
	'5': {"111", "100", "111", "001", "111"},
	'6': {"111", "100", "111", "101", "111"},
	'7': {"111", "001", "001", "001", "001"},
	'8': {"111", "101", "111", "101", "111"},
	'9': {"111", "101", "111", "001", "111"},
	':': {"0", "1", "0", "1", "0"},
	' ': {"0", "0", "0", "0", "0"},
	'A': {"010", "101", "111", "101", "101"},
	'P': {"110", "101", "110", "100", "100"},
	'M': {"10001", "11011", "10101", "10001", "10001"},
	'.': {"0", "0", "0", "0", "1"},
	',': {"0", "0", "0", "1", "1"},
	'-': {"000", "000", "111", "000", "000"},
	'%': {"101", "001", "010", "100", "101"},
	'K': {"101", "110", "100", "110", "101"},
	'G': {"111", "100", "101", "101", "111"},
	'B': {"110", "101", "110", "101", "110"},
	'T': {"111", "010", "010", "010", "010"},
}

// glyph is r's bitmap, or its uppercase form's when r has none of its own.
func glyph(r rune) ([5]string, bool) {
	if g, ok := glyphs[r]; ok {
		return g, true
	}
	g, ok := glyphs[unicode.ToUpper(r)]
	return g, ok
}

func DigitsSize(text string, d widget.Digits) widget.Size {
	if d == widget.NoDigits {
		return widget.Size{W: len([]rune(text)), H: 1}
	}
	scale, rows := 1, 3
	if d == widget.Block {
		scale, rows = 2, 5
	}
	w, n := 0, 0
	for _, r := range text {
		if g, ok := glyph(r); ok {
			w += len(g[0]) * scale
			n++
		}
	}
	if n > 1 {
		w += (n - 1) * scale
	}
	return widget.Size{W: w, H: rows}
}

func digitLines(th theme.Theme, l widget.Line) []string {
	scale, rows := 1, 3
	if l.Digits == widget.Block {
		scale, rows = 2, 5
	}
	out := make([]strings.Builder, rows)
	first := true
	for _, s := range l.Spans {
		st := th.Fg(s.Token)
		for _, r := range s.Text {
			g, ok := glyph(r)
			if !ok {
				continue
			}
			for row := range out {
				if !first {
					out[row].WriteString(strings.Repeat(" ", scale))
				}
				if l.Digits == widget.Block {
					out[row].WriteString(st.Render(blockRow(g[row])))
				} else {
					out[row].WriteString(st.Render(halfRow(g, row)))
				}
			}
			first = false
		}
	}
	lines := make([]string, rows)
	for i := range out {
		lines[i] = out[i].String()
	}
	return lines
}

func blockRow(dots string) string {
	var b strings.Builder
	for _, d := range dots {
		if d == '1' {
			b.WriteString("██")
		} else {
			b.WriteString("  ")
		}
	}
	return b.String()
}

// halfRow is cell row i of a half-block glyph: dot rows 2i and 2i+1.
func halfRow(g [5]string, i int) string {
	top, bottom := g[2*i], ""
	if 2*i+1 < len(g) {
		bottom = g[2*i+1]
	}
	var b strings.Builder
	for c := range len(top) {
		t, u := top[c] == '1', bottom != "" && bottom[c] == '1'
		switch {
		case t && u:
			b.WriteString("█")
		case t:
			b.WriteString("▀")
		case u:
			b.WriteString("▄")
		default:
			b.WriteString(" ")
		}
	}
	return b.String()
}
