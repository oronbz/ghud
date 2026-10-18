package host

import (
	"reflect"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/oronbz/ghud/internal/layout"
	"github.com/oronbz/ghud/internal/theme"
)

// doubleClick is how soon a second click on the same place counts as a double-click.
const doubleClick = 400 * time.Millisecond

// drag is a left-button drag of a title (moving its Widget) or of a seam
// (resizing the Widgets either side).
type drag struct {
	zone layout.Zone
	// dx, dy is where a title was grabbed from its tile's corner; for a
	// seam, dx (or dy) is the grab's offset from the seam's first cell.
	dx, dy int
	x, y   int
	moved  bool
	drop   *layout.Drop
	// before is the Layout a seam drag reflows from.
	before layout.Layout
}

// click is the last left click, to tell a double-click.
type click struct {
	zone layout.Zone
	at   time.Time
}

func (m Model) mouseMsg(msg tea.MouseMsg) (Model, tea.Cmd) {
	if !m.mouse || m.keyMap || m.overlay != nil || m.palette != nil || m.picking || m.arranging {
		return m, nil
	}
	ms := msg.Mouse()
	s := m.scene()
	z, hit := s.Hit(ms.X, ms.Y)
	switch msg.(type) {
	case tea.MouseMotionMsg:
		if m.drag != nil {
			return m.dragTo(s.Frame, ms.X, ms.Y), nil
		}
		m.hover = nil
		if hit && (z.Kind == layout.Title || z.Kind == layout.WidgetSeam || z.Kind == layout.RowSeam) {
			m.hover = &z
		}
	case tea.MouseClickMsg:
		if ms.Button == tea.MouseLeft && hit {
			return m.press(s, z, ms.X, ms.Y)
		}
	case tea.MouseReleaseMsg:
		return m.release(), nil
	}
	return m, nil
}

// pointer is the hover or drag the next frame draws; none while a modal
// view has the input.
func (m Model) pointer() layout.Pointer {
	var p layout.Pointer
	if m.keyMap || m.overlay != nil || m.palette != nil || m.picking || m.arranging {
		return p
	}
	if d := m.drag; d != nil {
		if d.zone.Kind != layout.Title {
			p.Seam = &d.zone
			return p
		}
		p.Handle = d.zone.ID
		if d.moved {
			p.Drag = &layout.Drag{ID: d.zone.ID, X: d.x, Y: d.y, DX: d.dx, DY: d.dy, Drop: d.drop}
		}
		return p
	}
	if h := m.hover; h != nil {
		if h.Kind == layout.Title {
			p.Handle = h.ID
		} else {
			p.Seam = h
		}
	}
	return p
}

// press handles a left click on zone z.
func (m Model) press(s scene, z layout.Zone, x, y int) (Model, tea.Cmd) {
	switch z.Kind {
	case layout.Title:
		t, _ := s.Tile(z.ID)
		m.focus, m.last = z.ID, z.ID
		m.drag = &drag{zone: z, dx: x - t.X, dy: y - t.Y, x: x, y: y}
	}
	return m, nil
}

// dragTo follows the pointer: a title drag tracks where it would drop.
func (m Model) dragTo(f layout.Frame, x, y int) Model {
	d := *m.drag
	d.x, d.y = x, y
	if d.zone.Kind == layout.Title {
		d.moved = true
		d.drop = nil
		if drop, ok := f.DropAt(d.zone.ID, x, y); ok {
			d.drop = &drop
		}
	}
	m.drag = &d
	return m
}

// release ends a drag, writing the Layout when it changed.
func (m Model) release() Model {
	d := m.drag
	m.drag = nil
	if d == nil {
		return m
	}
	before := m.layout
	if d.zone.Kind == layout.Title && d.drop != nil {
		m.layout = m.layout.Drop(d.zone.ID, *d.drop)
	}
	if d.zone.Kind != layout.Title {
		before = d.before
	}
	if !reflect.DeepEqual(m.layout, before) {
		m = m.saveLayout()
	}
	return m
}

// dragLayers draw a title drag: the dashed ghost holding the title, and the
// heavy bar on the side the Widget would drop.
func (m Model) dragLayers(s scene) []*lipgloss.Layer {
	g, bar := s.Ghost, s.Bar
	if g.W < 2 || g.H < 2 {
		return nil
	}
	line := m.opts.Theme.Fg(theme.Accent)
	edge := line.Render(strings.Repeat("┄", g.W-2))
	side := line.Render(strings.TrimSuffix(strings.Repeat("┆\n", g.H-2), "\n"))
	title := ansi.Truncate(" "+m.widgets[m.drag.zone.ID].title+" ", max(0, g.W-4), "…")
	layers := []*lipgloss.Layer{
		lipgloss.NewLayer(line.Render("╭") + edge + line.Render("╮")).X(g.X).Y(g.Y).Z(2),
		lipgloss.NewLayer(line.Render("╰") + edge + line.Render("╯")).X(g.X).Y(g.Y + g.H - 1).Z(2),
		lipgloss.NewLayer(m.opts.Theme.Fg(theme.Bright).Bold(true).Render(title)).X(g.X + 2).Y(g.Y).Z(3),
	}
	if g.H > 2 {
		layers = append(layers,
			lipgloss.NewLayer(side).X(g.X).Y(g.Y+1).Z(2),
			lipgloss.NewLayer(side).X(g.X+g.W-1).Y(g.Y+1).Z(2))
	}
	if bar.W > 0 {
		heavy := m.opts.Theme.Fg(theme.Accent).Bold(true)
		s := heavy.Render(strings.Repeat("━", bar.W))
		if bar.W == 1 {
			s = heavy.Render(strings.TrimSuffix(strings.Repeat("┃\n", bar.H), "\n"))
		}
		layers = append(layers, lipgloss.NewLayer(s).X(bar.X).Y(bar.Y).Z(4))
	}
	return layers
}

// tint lights the border cells in rs in the accent colour, leaving any text
// drawn over the border (a title) as it is.
func (m Model) tint(c *lipgloss.Canvas, rs []layout.Rect) {
	accent := m.opts.Theme.Color(theme.Accent)
	for _, r := range rs {
		for y := r.Y; y < r.Y+r.H; y++ {
			for x := r.X; x < r.X+r.W; x++ {
				cell := c.CellAt(x, y)
				if cell == nil || cell.Content != "─" && cell.Content != "│" {
					continue
				}
				lit := *cell
				lit.Style.Fg = accent
				c.SetCell(x, y, &lit)
			}
		}
	}
}
