package render

import (
	"strings"

	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

var halfBlockDigits = map[rune][3]string{
	'0': {"█▀█", "█ █", "▀▀▀"}, '1': {" ▀█", "  █", "  ▀"}, '2': {"▀▀█", "█▀▀", "▀▀▀"},
	'3': {"▀▀█", " ▀█", "▀▀▀"}, '4': {"█ █", "▀▀█", "  ▀"}, '5': {"█▀▀", "▀▀█", "▀▀▀"},
	'6': {"█▀▀", "█▀█", "▀▀▀"}, '7': {"▀▀█", "  █", "  ▀"}, '8': {"█▀█", "█▀█", "▀▀▀"},
	'9': {"█▀█", "▀▀█", "▀▀▀"}, ':': {" ", "▀", "▀"}, ' ': {" ", " ", " "},
}

func bigLines(th theme.Theme, l widget.Line) []string {
	var rows [3]strings.Builder
	for _, s := range l.Spans {
		st := th.Fg(s.Token)
		for _, r := range s.Text {
			glyph, ok := halfBlockDigits[r]
			if !ok {
				continue
			}
			for i := range rows {
				if rows[i].Len() > 0 {
					rows[i].WriteByte(' ')
				}
				rows[i].WriteString(st.Render(glyph[i]))
			}
		}
	}
	return []string{rows[0].String(), rows[1].String(), rows[2].String()}
}
