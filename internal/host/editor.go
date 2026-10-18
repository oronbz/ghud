package host

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/oronbz/ghud/internal/theme"
)

// editing is INSERT mode: the host's multi-line editor over a Widget's body,
// editing an InlineInput Action's Value.
type editing struct {
	widget string
	action bound
	ta     textarea.Model
	// start is the text as the editor first held it, so an untouched edit writes nothing.
	start string
}

// insertHostKeys stay host keys in INSERT mode rather than reaching the editor.
var insertHostKeys = []string{"ctrl+k"}

// maxEditLines is the textarea's own line cap; a longer note would be cut on save.
const maxEditLines = 10000

func (m Model) startEditing(id string, a bound) (Model, tea.Cmd) {
	if strings.Count(a.Value, "\n") >= maxEditLines {
		return m.say("Too long to edit here; E opens $EDITOR", theme.Warn)
	}
	th := m.opts.Theme
	st := textarea.StyleState{
		Base:        lipgloss.NewStyle(),
		Text:        th.Fg(theme.Fg),
		CursorLine:  th.Fg(theme.Fg),
		EndOfBuffer: th.Fg(theme.Faint),
		Placeholder: th.Fg(theme.Faint),
		Selection:   th.Fg(theme.Bright).Background(th.Color(theme.Sel)),
	}
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.MaxHeight = 0
	// ctrl+v would read the system clipboard behind Ports' back; terminal pastes arrive as PasteMsg.
	ta.KeyMap.Paste.SetEnabled(false)
	ta.SetStyles(textarea.Styles{Focused: st, Blurred: st, Cursor: textarea.CursorStyle{Color: th.Color(theme.Accent), Shape: tea.CursorBlock}})
	ta.SetValue(a.Value)
	ta.Focus()
	m.editing = &editing{widget: id, action: a, ta: ta, start: ta.Value()}
	m.sizeEditor()
	return m, nil
}

// sizeEditor fits the editor to its Widget's body, inside the one-cell margins.
func (m Model) sizeEditor() {
	e := m.editing
	if e == nil {
		return
	}
	if r, ok := m.rects()[e.widget]; ok {
		e.ta.SetWidth(max(1, r.W-4))
		e.ta.SetHeight(max(1, r.H-2))
	}
}

// editKey types into the editor; esc leaves INSERT mode, saving a changed text.
func (m Model) editKey(msg tea.Msg) (Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "esc" {
		return m.saveEdit()
	}
	e := *m.editing
	var cmd tea.Cmd
	e.ta, cmd = e.ta.Update(msg)
	m.editing = &e
	return m, cmd
}

// saveEdit leaves INSERT mode, saving a changed text.
func (m Model) saveEdit() (Model, tea.Cmd) {
	e := *m.editing
	m.editing = nil
	if text := e.ta.Value(); text != e.start {
		return m.invoke(e.widget, e.action, text)
	}
	return m, nil
}

func editsKey(msg tea.KeyPressMsg) bool { return !slices.Contains(insertHostKeys, msg.String()) }

// editorLayer draws the editor over the body of the tile at x, y.
func (m Model) editorLayer(x, y int) *lipgloss.Layer {
	return lipgloss.NewLayer(m.editing.ta.View()).X(x + 2).Y(y + 1).Z(1)
}
