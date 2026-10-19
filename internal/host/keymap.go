package host

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/oronbz/ghud/internal/render"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

func (m Model) keyMapLayer() *lipgloss.Layer {
	bs := Bindings()
	keyW, descW := 0, 0
	for _, b := range bs {
		keyW = max(keyW, ansi.StringWidth(b.Help().Key))
		descW = max(descW, ansi.StringWidth(b.Help().Desc))
	}
	lines := make([]widget.Line, 0, len(bs))
	for _, b := range bs {
		h := b.Help()
		lines = append(lines, widget.Line{Spans: []widget.Span{
			{Text: " " + h.Key + strings.Repeat(" ", keyW-ansi.StringWidth(h.Key)+2), Token: theme.Accent, Bold: true},
			{Text: h.Desc + " ", Token: theme.Fg},
		}})
	}
	w := min(m.w, keyW+descW+6)
	h := min(m.h, len(lines)+2)
	box := render.Draw(m.opts.Theme, render.Tile{Title: "Keys", W: w, H: h, Focused: true, Payload: widget.Payload{Lines: lines}})
	return lipgloss.NewLayer(box).X((m.w - w) / 2).Y((m.h - h) / 2).Z(1)
}
