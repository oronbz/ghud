package host

import (
	"context"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

// choicesMsg brings the choices of the choice menu numbered seq.
type choicesMsg struct {
	seq     int
	choices []widget.Choice
	err     error
}

// choose opens choice Action a's menu at once, with a spinner, and asks the
// Widget for its choices.
func (m Model) choose(id string, a bound) (Model, tea.Cmd) {
	chooser, ok := m.widgets[id].widget.(widget.Chooser)
	if !ok {
		return m, nil
	}
	m.menus++
	x, y := m.selectedAt(id)
	pending := a
	pending.Input, pending.Confirm = widget.NoInput, false
	m.menu = &menu{widget: id, title: promptOf(a), x: x, y: y, loading: true, seq: m.menus, choosing: pending}
	inv := widget.Invocation{Action: a.ID, Item: a.item, Base: a.Value, Now: m.opts.Ports.Clock.Now()}
	seq := m.menus
	load := func() tea.Msg {
		cs, err := chooser.Choices(context.Background(), inv)
		return choicesMsg{seq: seq, choices: cs, err: err}
	}
	if m.spinning {
		return m, load
	}
	m.spinning = true
	return m, tea.Batch(load, m.opts.Ports.Clock.After(spinEvery, func(t time.Time) tea.Msg { return spinMsg{t} }))
}

// chose fills the choice menu still waiting for msg, or closes it with why
// there's nothing to pick.
func (m Model) chose(msg choicesMsg) (Model, tea.Cmd) {
	if m.menu == nil || !m.menu.loading || m.menu.seq != msg.seq {
		return m, nil
	}
	a := m.menu.choosing
	switch {
	case msg.err != nil:
		m.menu = nil
		return m.say(capitalise(a.Label)+" failed: "+msg.err.Error(), theme.Bad)
	case len(msg.choices) == 0:
		m.menu = nil
		return m.say("Nothing to "+uncapitalise(promptOf(a)), theme.Fg)
	}
	mu := *m.menu
	mu.loading = false
	for _, c := range msg.choices {
		e := a
		e.choice = &c
		if c.Target != "" {
			e.Verb, e.Target = widget.Open, c.Target
		}
		mu.actions = append(mu.actions, e)
	}
	m.menu = &mu
	return m, nil
}

// promptOf titles a choice Action's menu: its Prompt, else its label.
func promptOf(a bound) string {
	if a.Prompt != "" {
		return a.Prompt
	}
	return capitalise(a.Label)
}

func uncapitalise(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToLower(r)) + s[n:]
}
