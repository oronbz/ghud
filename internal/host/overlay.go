package host

import (
	"slices"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/oronbz/ghud/internal/render"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

type overlayKind int

const (
	confirming overlayKind = iota
	prompting
)

// overlay asks before an Action runs: a yes/no confirm, or a one-line text prompt.
type overlay struct {
	kind   overlayKind
	widget string
	action bound
	value  []rune
	cursor int
}

func (m Model) overlayKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	o := *m.overlay
	k := msg.String()
	if k == "esc" || o.kind == confirming && k == "n" {
		m.overlay = nil
		return m, nil
	}
	if o.kind == confirming {
		if k == "enter" || k == "y" {
			m.overlay = nil
			if o.action.Input == widget.TextInput {
				o.action.Confirm = false
				return m.run(o.widget, o.action)
			}
			return m, m.invoke(o.widget, o.action, "")
		}
		return m, nil
	}
	switch k {
	case "enter":
		m.overlay = nil
		return m, m.invoke(o.widget, o.action, string(o.value))
	case "left":
		o.cursor = max(0, o.cursor-1)
	case "right":
		o.cursor = min(len(o.value), o.cursor+1)
	case "home", "ctrl+a":
		o.cursor = 0
	case "end", "ctrl+e":
		o.cursor = len(o.value)
	case "backspace":
		if o.cursor > 0 {
			o.value = slices.Delete(slices.Clone(o.value), o.cursor-1, o.cursor)
			o.cursor--
		}
	case "delete", "ctrl+d":
		if o.cursor < len(o.value) {
			o.value = slices.Delete(slices.Clone(o.value), o.cursor, o.cursor+1)
		}
	case "ctrl+u":
		o.value = slices.Clone(o.value[o.cursor:])
		o.cursor = 0
	default:
		if msg.Text != "" {
			r := []rune(msg.Text)
			o.value = slices.Insert(slices.Clone(o.value), o.cursor, r...)
			o.cursor += len(r)
		}
	}
	m.overlay = &o
	return m, nil
}

// overlayLayer draws the confirm or prompt box centred over the dashboard.
func (m Model) overlayLayer() *lipgloss.Layer {
	o := m.overlay
	w := min(76, m.w-6)
	var title string
	var line widget.Line
	var hints []render.Hint
	switch o.kind {
	case confirming:
		title = capitalise(o.action.Label) + "?"
		line = widget.Line{Spans: []widget.Span{{Text: " " + o.action.subject, Token: theme.Bright}}}
		hints = []render.Hint{{Key: "enter", Label: o.action.Label}, {Key: "esc", Label: "cancel"}}
	case prompting:
		title = o.action.Prompt
		if title == "" {
			title = capitalise(o.action.Label)
		}
		line = promptLine(o, w-6)
		hints = []render.Hint{{Key: "enter", Label: o.action.Label}, {Key: "esc", Label: "cancel"}}
	}
	box := render.Draw(m.opts.Theme, render.Tile{
		Title: title, W: w, H: 5, Focused: true,
		Payload: widget.Payload{Lines: []widget.Line{{}, line, {}}},
		Hints:   hints,
	})
	return lipgloss.NewLayer(box).X((m.w - w) / 2).Y(max(0, m.h/2-3)).Z(1)
}

// promptLine is "› value" with the cursor cell in accent, scrolled to keep
// the cursor in its w cells.
func promptLine(o *overlay, w int) widget.Line {
	v, c := o.value, o.cursor
	if start := c - (w - 1); start > 0 {
		v, c = v[start:], c-start
	}
	cur := " "
	after := ""
	if c < len(v) {
		cur, after = string(v[c]), string(v[c+1:])
	}
	return widget.Line{Spans: []widget.Span{
		{Text: " › ", Token: theme.Accent},
		{Text: string(v[:c]), Token: theme.Bright},
		{Text: cur, Token: theme.Bg, Bg: theme.Accent},
		{Text: after, Token: theme.Bright},
	}}
}
