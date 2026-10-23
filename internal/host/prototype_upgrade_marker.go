package host

// PROTOTYPE for #75, throwaway: where and how the Upgrade marker sits in the
// footer, and the Upgrade popup. F9 cycles the marker variants
// (VISOR_PROTOTYPE_MARKER=A|B|C|off picks the first); F10 reopens the popup and
// cycles its placement (centre, corner). Every variant pretends v0.7.0 is out.

import (
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"charm.land/lipgloss/v2"

	"github.com/oronbz/visor/internal/render"
	"github.com/oronbz/visor/internal/theme"
	"github.com/oronbz/visor/internal/widget"
)

var protoVariants = []string{"A", "B", "C", "off"}

var protoNames = map[string]string{
	"A":   "accent text",
	"B":   "badge, like the mode badge",
	"C":   "key hint, shrinks when narrow",
	"off": "no marker",
}

var protoVariant = func() int {
	for i, v := range protoVariants {
		if strings.EqualFold(v, os.Getenv("VISOR_PROTOTYPE_MARKER")) {
			return i
		}
	}
	return 1
}()

var (
	protoPopupOpen  = true
	protoPopupPlace = 0
	protoPlaces     = []string{"centre modal", "corner card above the marker"}
)

func (m Model) protoKey(k string) (Model, tea.Cmd, bool) {
	switch {
	case k == "f9":
		m, cmd := m.protoCycle()
		return m, cmd, true
	case k == "f10":
		if protoPopupOpen {
			protoPopupPlace = (protoPopupPlace + 1) % len(protoPlaces)
		}
		protoPopupOpen = true
		m, cmd := m.say("prototype popup: "+protoPlaces[protoPopupPlace], theme.Fg)
		return m, cmd, true
	case !protoPopupOpen:
		return m, nil, false
	case k == "enter":
		protoPopupOpen = false
		m, cmd := m.say("prototype: would leave the screen and run brew upgrade --cask visor, then come back as v0.7.0", theme.Fg)
		return m, cmd, true
	case k == "esc":
		protoPopupOpen = false
		m, cmd := m.say("prototype: v0.7.0 dismissed; ↑ U still upgrades", theme.Fg)
		return m, cmd, true
	case k == "o":
		m, cmd := m.say("prototype: would open github.com/oronbz/visor/releases/tag/v0.7.0", theme.Fg)
		return m, cmd, true
	}
	return m, nil, true
}

func (m Model) protoPopupLayer() *lipgloss.Layer {
	if !protoPopupOpen {
		return nil
	}
	lines := []widget.Line{
		{Spans: []widget.Span{{Text: " A new release of visor is out. You have ", Token: theme.Fg}, {Text: "v0.6.2", Token: theme.Dim}, {Text: ".", Token: theme.Fg}}},
		widget.Text(theme.Fg, ""),
		{Spans: []widget.Span{{Text: " What's new", Token: theme.Bright, Bold: true}}},
		widget.Text(theme.Fg, " • Jira: an issue's glyph is its type, in Jira's colours"),
		widget.Text(theme.Fg, " • PRs: a token missing scopes asks for setup"),
		widget.Text(theme.Fg, " • Upgrade from the footer marker or U"),
		widget.Text(theme.Fg, ""),
		{Spans: []widget.Span{{Text: " ⏎ runs ", Token: theme.Dim}, {Text: "brew upgrade --cask visor", Token: theme.Accent}}},
	}
	w := min(60, m.w-4)
	h := min(m.h-footerHeight, len(lines)+2)
	box := render.Draw(m.opts.Theme, render.Tile{
		Title: "visor v0.7.0", W: w, H: h, Focused: true,
		Payload: widget.Payload{Lines: lines},
		Hints:   []render.Hint{{Key: "enter", Label: "upgrade"}, {Key: "o", Label: "release notes"}, {Key: "esc", Label: "later"}},
	})
	if protoPopupPlace == 1 {
		return lipgloss.NewLayer(box).X(m.w - w).Y(max(0, m.h-footerHeight-h)).Z(2)
	}
	return lipgloss.NewLayer(box).X((m.w - w) / 2).Y(max(0, (m.h-h)/3)).Z(2)
}

func (m Model) protoCycle() (Model, tea.Cmd) {
	protoVariant = (protoVariant + 1) % len(protoVariants)
	v := protoVariants[protoVariant]
	return m.say("prototype marker "+v+": "+protoNames[v], theme.Fg)
}

func (m Model) protoMarker() string {
	th := m.opts.Theme
	switch protoVariants[protoVariant] {
	case "A":
		return th.Fg(theme.Accent).Bold(true).Render("↑ v0.7.0")
	case "B":
		return th.Fg(theme.Bg).Background(th.Color(theme.Accent)).Bold(true).Render(" ↑ v0.7.0 ")
	case "C":
		if m.w < 90 {
			return th.Fg(theme.Accent).Bold(true).Render("U") + th.Fg(theme.Accent).Render(" ↑ v0.7.0")
		}
		return th.Fg(theme.Accent).Bold(true).Render("U") + " " + th.Fg(theme.Dim).Render("upgrade to") + " " + th.Fg(theme.Accent).Bold(true).Render("v0.7.0")
	}
	return ""
}

// protoFooter pins the marker to the right edge and cuts the rest before it.
func (m Model) protoFooter(left string) string {
	marker := m.protoMarker()
	if marker == "" {
		return ansi.Truncate(left, m.w, "…")
	}
	mw := ansi.StringWidth(marker)
	room := m.w - mw - 1
	if room < 1 {
		return ansi.Truncate(marker, m.w, "")
	}
	left = ansi.Truncate(left, room, "…")
	return left + strings.Repeat(" ", m.w-ansi.StringWidth(left)-mw) + marker
}
