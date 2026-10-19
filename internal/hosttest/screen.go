package hosttest

import (
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/oronbz/ghud/internal/host"
	"github.com/oronbz/ghud/internal/layout"
	"github.com/oronbz/ghud/internal/theme"
)

// View is the app's current tea.View, for the terminal state it declares
// (background colour, mouse mode, keyboard flags).
func (h *Host) View() tea.View { return h.model.View() }

// Theme is the palette the app draws with now; Fg and Bg map cells back through it.
func (h *Host) Theme() theme.Theme { return h.model.(host.Model).Theme() }

// Buffer re-parses the current frame into a cell grid of the terminal size.
func (h *Host) Buffer() uv.ScreenBuffer {
	buf := uv.NewScreenBuffer(h.w, h.h)
	uv.NewStyledString(h.model.View().Content).Draw(buf, buf.Bounds())
	return buf
}

// Zones are the hit zones of the current frame, from the same layout pass that drew it.
func (h *Host) Zones() []layout.Zone { return h.model.(host.Model).Zones() }

// Screen is the current frame as plain text, one line per row, trailing spaces trimmed.
func (h *Host) Screen() string {
	buf := h.Buffer()
	lines := make([]string, buf.Height())
	for y := range lines {
		var b strings.Builder
		for x := 0; x < buf.Width(); {
			c := buf.CellAt(x, y)
			if c == nil || c.Content == "" {
				b.WriteByte(' ')
				x++
				continue
			}
			b.WriteString(c.Content)
			x += max(c.Width, 1)
		}
		lines[y] = strings.TrimRight(b.String(), " ")
	}
	return strings.Join(lines, "\n")
}

func (h *Host) Line(y int) string {
	lines := strings.Split(h.Screen(), "\n")
	if y < 0 || y >= len(lines) {
		return ""
	}
	return lines[y]
}

func (h *Host) Cell(x, y int) *uv.Cell { return h.Buffer().CellAt(x, y) }

// Find returns the cell position of the first occurrence of text on screen.
func (h *Host) Find(text string) (x, y int, ok bool) {
	for y, line := range strings.Split(h.Screen(), "\n") {
		if i := strings.Index(line, text); i >= 0 {
			return len([]rune(line[:i])), y, true
		}
	}
	return 0, 0, false
}

// Fg names the theme token of the cell's foreground colour; ok is false for
// colours outside the theme, so asserts survive palette changes.
func (h *Host) Fg(x, y int) (tok theme.Token, ok bool) {
	c := h.Cell(x, y)
	if c == nil {
		return 0, false
	}
	return h.token(c.Style.Fg)
}

// Bg names the theme token of the cell's background colour, like Fg.
func (h *Host) Bg(x, y int) (tok theme.Token, ok bool) {
	c := h.Cell(x, y)
	if c == nil {
		return 0, false
	}
	return h.token(c.Style.Bg)
}

func (h *Host) Bold(x, y int) bool {
	c := h.Cell(x, y)
	return c != nil && c.Style.Attrs&uv.AttrBold != 0
}

func (h *Host) token(c color.Color) (theme.Token, bool) {
	if c == nil {
		return 0, false
	}
	for _, tok := range theme.Tokens() {
		if sameColor(h.Theme().Color(tok), c) {
			return tok, true
		}
	}
	return 0, false
}

func sameColor(a, b color.Color) bool {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	return ar>>8 == br>>8 && ag>>8 == bg>>8 && ab>>8 == bb>>8
}
