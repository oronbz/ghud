package host

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/oronbz/ghud/internal/theme"
)

// footerHeight is the one line under the grid for the mode badge and hints.
const footerHeight = 1

func (m Model) footer() string {
	th := m.opts.Theme
	mode, modeColor := " NORMAL ", theme.Accent
	bindings := normalBindings()
	hints := "Tab/hl focus · 1–9 jump · w widgets · ? keys · q quit"
	if m.focus != "" {
		hints = "Tab rows · hl widgets · HJKL any direction · m arrange · esc unfocus · ? keys"
	}
	if m.arranging {
		mode, modeColor = " ARRANGE ", theme.Warn
		bindings = arrangeBindings()
		hints = "hjkl move · HJKL resize · ⏎ keep · esc revert"
	}
	if m.picking {
		hints = "jk move · ␣ show/hide · esc close"
	}
	if m.editing != nil {
		mode, modeColor = " INSERT ", theme.Ok
		hints = "esc save"
	}
	if m.palette != nil {
		hints = "type to filter · ↑↓ choose · ⏎ run · esc close"
	}
	if m.menu != nil {
		hints = "jk choose · ⏎ run · esc close"
	}
	if o := m.overlay; o != nil {
		mode, modeColor = " INPUT ", theme.Ok
		if o.kind == confirming {
			mode, modeColor = " CONFIRM ", theme.Warn
		}
		hints = "⏎ " + o.action.Label + " · esc cancel"
	}
	badge := th.Fg(theme.Bg).Background(th.Color(modeColor)).Bold(true).Render(mode)
	switch m.held {
	case tea.ModShift:
		hints = modifierHints("Shift", bindings, isShifted)
	case tea.ModCtrl:
		hints = modifierHints("Ctrl", bindings, func(k string) bool { return strings.HasPrefix(k, "ctrl+") })
	}
	toast := ""
	if m.toast.text != "" {
		toast = th.Fg(m.toast.tok).Render(m.toast.text) + "  "
	}
	return ansi.Truncate(badge+" "+toast+th.Fg(theme.Dim).Render(hints), m.w, "…")
}

// modifierHints lists the bindings that have at least one key using the modifier.
func modifierHints(name string, bindings []key.Binding, uses func(key string) bool) string {
	var hints []string
	for _, b := range bindings {
		for _, k := range b.Keys() {
			if uses(k) {
				hints = append(hints, b.Help().Key+" "+b.Help().Desc)
				break
			}
		}
	}
	if hints == nil {
		return name + ": no keys"
	}
	return name + ": " + strings.Join(hints, " · ")
}

func isShifted(k string) bool {
	r, size := utf8.DecodeRuneInString(k)
	return strings.Contains(k, "shift+") || size == len(k) && unicode.IsUpper(r)
}

// loneModifier reports whether code is a modifier key pressed on its own,
// and the Shift or Ctrl it swaps the footer to (0 for the others).
func loneModifier(code rune) (tea.KeyMod, bool) {
	switch code {
	case tea.KeyLeftShift, tea.KeyRightShift:
		return tea.ModShift, true
	case tea.KeyLeftCtrl, tea.KeyRightCtrl:
		return tea.ModCtrl, true
	case tea.KeyLeftAlt, tea.KeyRightAlt, tea.KeyLeftSuper, tea.KeyRightSuper,
		tea.KeyLeftHyper, tea.KeyRightHyper, tea.KeyLeftMeta, tea.KeyRightMeta,
		tea.KeyIsoLevel3Shift, tea.KeyIsoLevel5Shift:
		return 0, true
	}
	return 0, false
}
