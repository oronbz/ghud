package host

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/oronbz/ghud/internal/layout"
	"github.com/oronbz/ghud/internal/render"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

// menu is the Action menu of one Widget's selected Item, opened by
// right-click or ".", anchored at the pointer or the Item.
type menu struct {
	widget, title string
	actions       []bound
	sel           int
	x, y          int
}

// openMenu opens Widget id's Action menu with its top-left corner at x, y,
// titled with the selected Item's name, or the Widget's when there's none.
func (m Model) openMenu(id string, x, y int) Model {
	as := m.actions(id)
	if len(as) == 0 {
		return m
	}
	title := m.widgets[id].title
	if as[0].item != "" && as[0].subject != "" {
		title = as[0].subject
	}
	m.menu = &menu{widget: id, title: title, actions: as, x: x, y: y}
	return m
}

// selectedAt is where a keyboard-opened menu goes: just inside the selected
// Item's line, or the Widget's top-left inner corner.
func (m Model) selectedAt(id string) (x, y int) {
	s := m.scene()
	idx := m.lists[id].idx
	for _, z := range s.Zones {
		if z.ID == id && z.Kind == layout.Item && z.Index == idx {
			return z.X + 2, z.Y + 1
		}
	}
	for _, z := range s.Zones {
		if z.ID == id && z.Kind == layout.Body {
			return z.X + 1, z.Y
		}
	}
	return 0, 0
}

func (m Model) menuKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	mu := *m.menu
	switch msg.String() {
	case "esc", ".":
		m.menu = nil
		return m, nil
	case "enter":
		m.menu = nil
		return m.run(mu.widget, mu.actions[mu.sel])
	case "j", "down", "ctrl+n":
		mu.sel = min(mu.sel+1, len(mu.actions)-1)
	case "k", "up", "ctrl+p":
		mu.sel = max(mu.sel-1, 0)
	}
	m.menu = &mu
	return m, nil
}

// menuMouse runs the clicked entry, follows the pointer over entries, and
// closes the menu on a click anywhere else.
func (m Model) menuMouse(msg tea.MouseMsg) (Model, tea.Cmd) {
	ms := msg.Mouse()
	box := m.menuBox()
	i := ms.Y - box.Y - 1
	on := ms.X > box.X && ms.X < box.X+box.W-1 && i >= 0 && i < len(m.menu.actions)
	switch msg.(type) {
	case tea.MouseMotionMsg:
		if on {
			mu := *m.menu
			mu.sel = i
			m.menu = &mu
		}
	case tea.MouseClickMsg:
		mu := *m.menu
		m.menu = nil
		if on && ms.Button == tea.MouseLeft {
			return m.run(mu.widget, mu.actions[i])
		}
	}
	return m, nil
}

// menuBox is the menu's outer rectangle: as wide as its widest entry, kept
// on screen above the footer.
func (m Model) menuBox() layout.Rect {
	mu := m.menu
	w := ansi.StringWidth(mu.title) + 6
	for _, a := range mu.actions {
		w = max(w, menuLabelW(mu)+ansi.StringWidth(render.KeyLabel(shownKey(a)))+6)
	}
	w = min(w, m.w)
	h := min(len(mu.actions)+2, m.h-footerHeight)
	return layout.Rect{
		X: min(max(mu.x, 0), m.w-w),
		Y: min(max(mu.y, 0), m.h-footerHeight-h),
		W: w, H: h,
	}
}

func menuLabelW(mu *menu) int {
	n := 0
	for _, a := range mu.actions {
		n = max(n, ansi.StringWidth(a.Label))
	}
	return n
}

// menuLayer draws each entry as its label and the key that runs it, the
// selected one on the sel background.
func (m Model) menuLayer() *lipgloss.Layer {
	mu := m.menu
	box := m.menuBox()
	labelW := menuLabelW(mu)
	var lines []widget.Line
	for i, a := range mu.actions {
		bg, tok := theme.Bg, theme.Fg
		if i == mu.sel {
			bg, tok = theme.Sel, theme.Bright
		}
		key := render.KeyLabel(shownKey(a))
		gap := strings.Repeat(" ", max(0, box.W-4-labelW-ansi.StringWidth(key)))
		lines = append(lines, widget.Line{Spans: []widget.Span{
			{Text: " " + a.Label + strings.Repeat(" ", labelW-ansi.StringWidth(a.Label)), Token: tok, Bold: i == mu.sel, Bg: bg},
			{Text: gap, Bg: bg},
			{Text: key + " ", Token: theme.Accent, Bold: true, Bg: bg},
		}})
	}
	tile := render.Draw(m.opts.Theme, render.Tile{Title: mu.title, W: box.W, H: box.H, Focused: true, Payload: widget.Payload{Lines: lines}})
	return lipgloss.NewLayer(tile).X(box.X).Y(box.Y).Z(1)
}
