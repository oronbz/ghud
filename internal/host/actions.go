package host

import (
	"context"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/oronbz/ghud/internal/render"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

// toastLife is how long an Action's toast stays in the footer.
const toastLife = 5 * time.Second

type toast struct {
	text string
	tok  theme.Token
	// seq tells an expiring toast from the one that replaced it; zero never expires.
	seq int
}

type toastExpiredMsg struct{ seq int }

// say shows text in the footer for toastLife.
func (m Model) say(text string, tok theme.Token) (Model, tea.Cmd) {
	seq := m.toast.seq + 1
	m.toast = toast{text: text, tok: tok, seq: seq}
	return m, m.opts.Ports.Clock.After(toastLife, func(time.Time) tea.Msg { return toastExpiredMsg{seq} })
}

// listState is a list Widget's selection, kept by Item id so it survives a
// refresh, and its scroll offset, clamped here rather than when drawing.
type listState struct {
	sel      string
	idx, top int
}

func (s listState) render() render.List { return render.List{Selected: s.idx, Top: s.top} }

type refreshedMsg struct {
	id      string
	payload widget.Payload
	err     error
}

// editedMsg says $EDITOR exited after editing file for Widget id.
type editedMsg struct {
	id, file string
	err      error
	// then is the Action to run now, on the Item the edit began on.
	then *widget.Action
	item string
}

type actedMsg struct {
	id, item string
	action   widget.Action
	result widget.Result
	err    error
	// byHost marks an Action the host performed, which leaves the Widget as it was.
	byHost bool
}

// refresh fetches a Source's Payload off the UI thread; other Widgets need none.
func (m Model) refresh(id string) tea.Cmd {
	src, ok := m.widgets[id].widget.(widget.Source)
	if !ok {
		return nil
	}
	now := m.opts.Ports.Clock.Now()
	return func() tea.Msg {
		p, err := src.Refresh(context.Background(), now)
		return refreshedMsg{id: id, payload: p, err: err}
	}
}

type refreshDueMsg struct {
	id  string
	seq int
}

// schedule arms Widget id's next refresh after its type's interval. A newer
// schedule (after an Action's refresh) supersedes the armed one.
func (m Model) schedule(id string) tea.Cmd {
	d := m.widgets[id].typ.Refresh
	if d <= 0 {
		return nil
	}
	m.due[id]++
	seq := m.due[id]
	return m.opts.Ports.Clock.After(d, func(time.Time) tea.Msg { return refreshDueMsg{id: id, seq: seq} })
}

func (m Model) refreshDue(msg refreshDueMsg) tea.Cmd {
	if m.due[msg.id] != msg.seq {
		return nil
	}
	return m.refresh(msg.id)
}

func (m Model) refreshed(msg refreshedMsg) (Model, tea.Cmd) {
	next := m.schedule(msg.id)
	if msg.err != nil {
		m, cmd := m.say(m.widgets[msg.id].title+": "+msg.err.Error(), theme.Bad)
		return m, tea.Batch(cmd, next)
	}
	m.fetched[msg.id] = msg.payload
	st := m.lists[msg.id]
	if i := slices.IndexFunc(msg.payload.Items, func(it widget.Item) bool { return it.ID == st.sel }); i >= 0 {
		st.idx = i
	}
	m.lists[msg.id] = st
	m.clampList(msg.id)
	return m, next
}

// clampList keeps the selection on an Item and in view, its group header
// too when there's room, and the scroll offset inside the list.
func (m Model) clampList(id string) {
	r, ok := m.rects()[id]
	if !ok {
		return
	}
	p := m.payloadIn(id, r)
	st := m.lists[id]
	h := max(1, r.H-2)
	if p.Wrap && !p.IsList() {
		m.lists[id] = listState{top: min(max(st.top, 0), max(0, render.TextRows(p, r.W-2)-h))}
		return
	}
	if len(p.Items) == 0 {
		m.lists[id] = listState{}
		return
	}
	st.idx = min(max(st.idx, 0), len(p.Items)-1)
	st.sel = p.Items[st.idx].ID
	rows := render.Rows(p)
	line := slices.Index(rows, st.idx)
	want := line
	if line > 0 && rows[line-1] == render.Header && h > 1 {
		want = line - 1
	}
	if want < st.top {
		st.top = want
	}
	if line >= st.top+h {
		st.top = line - h + 1
	}
	st.top = min(max(st.top, 0), max(0, len(rows)-h))
	m.lists[id] = st
}

// moveSelection moves the focused list's selection to index i, clamped. A
// wrapped text Widget scrolls instead, i being its top line out of n.
func (m Model) moveSelection(i func(st listState, n int) int) {
	id := m.focus
	r, ok := m.rects()[id]
	if !ok {
		return
	}
	p := m.payloadIn(id, r)
	if p.Wrap && !p.IsList() {
		m.lists[id] = listState{top: i(listState{idx: m.lists[id].top}, render.TextRows(p, r.W-2))}
		m.clampList(id)
		return
	}
	if len(p.Items) == 0 {
		return
	}
	st := m.lists[id]
	st.idx = min(max(i(st, len(p.Items)), 0), len(p.Items)-1)
	st.sel = p.Items[st.idx].ID
	m.lists[id] = st
	m.clampList(id)
}

// bound is an Action as it applies now: to the selected Item, or to the Widget.
type bound struct {
	widget.Action
	item string
	// subject names the Item in confirm questions.
	subject string
	// by is the host-provided Action this is, or none for the Widget's own.
	by hostAction
}

type hostAction int

const (
	none hostAction = iota
	hide
)

// actions are Widget id's Actions as they apply now: the selected Item's,
// then the Widget's, then the host-provided ones.
func (m Model) actions(id string) []bound {
	r, ok := m.rects()[id]
	if !ok {
		return nil
	}
	host := []bound{{Action: widget.Action{ID: "hide", Label: "hide"}, by: hide}}
	if m.widgets[id].setup != "" {
		return host
	}
	p := m.payloadIn(id, r)
	var out []bound
	if st := m.lists[id]; st.idx < len(p.Items) {
		it := p.Items[st.idx]
		for _, a := range it.Actions {
			out = append(out, bound{Action: a, item: it.ID, subject: subject(p, it)})
		}
	}
	for _, a := range p.Actions {
		out = append(out, bound{Action: a})
	}
	return append(out, host...)
}

// subject is the text of an Item's first flex Field, the name it goes by.
func subject(p widget.Payload, it widget.Item) string {
	for c, col := range p.Columns {
		if col.Flex && c < len(it.Fields) {
			var b strings.Builder
			for _, s := range it.Fields[c] {
				b.WriteString(s.Text)
			}
			return b.String()
		}
	}
	return ""
}

// hints are the focused Widget's Action keys for its bottom border; Actions
// a host key shadows are left out.
func (m Model) hints(id string) []render.Hint {
	var hs []render.Hint
	for _, a := range m.actions(id) {
		if k := shownKey(a); k != "" {
			hs = append(hs, render.Hint{Key: k, Label: a.Label})
		}
	}
	return hs
}

// shownKey is the key that runs a while its Widget is focused, or "" when it
// has none or a host key shadows it.
func shownKey(a bound) string {
	k := a.Key
	if k == "" && a.Primary {
		k = "enter"
	}
	if k == "" || hostKey(k) && !(k == "enter" && a.Primary) {
		return ""
	}
	return k
}

// actionFor is the focused Widget's Action bound to key, Item Actions first.
func (m Model) actionFor(key string) (bound, bool) {
	for _, a := range m.actions(m.focus) {
		if a.Key == key || key == "enter" && a.Primary {
			return a, true
		}
	}
	return bound{}, false
}

// run starts an Action: it asks first when it must, then invokes it.
func (m Model) run(id string, a bound) (Model, tea.Cmd) {
	switch {
	case a.Confirm:
		m.overlay = &overlay{kind: confirming, widget: id, action: a}
	case a.Input == widget.TextInput:
		m.overlay = &overlay{kind: prompting, widget: id, action: a, value: []rune(a.Value), cursor: utf8.RuneCountInString(a.Value)}
	case a.Input == widget.InlineInput:
		return m.startEditing(id, a)
	default:
		return m.invoke(id, a, "")
	}
	return m, nil
}

// invoke performs host-provided Actions itself and hands the rest to the Widget.
func (m Model) invoke(id string, a bound, text string) (Model, tea.Cmd) {
	switch {
	case a.by == hide:
		return m.setHidden(id, true), nil
	case a.Verb == widget.Open:
		opener, target := m.opts.Ports.Opener, a.Target
		return m, func() tea.Msg {
			return actedMsg{id: id, action: a.Action, err: opener.Open(target), byHost: true}
		}
	case a.Verb == widget.Copy:
		m, toast := m.say("Copied to the clipboard", theme.Fg)
		return m, tea.Batch(m.opts.Ports.Clipboard.Copy(a.Target), toast)
	}
	actor, ok := m.widgets[id].widget.(widget.Actor)
	if !ok {
		return m, nil
	}
	inv := widget.Invocation{Action: a.ID, Item: a.item, Text: text, Base: a.Value, Now: m.opts.Ports.Clock.Now()}
	return m, func() tea.Msg {
		res, err := actor.Invoke(context.Background(), inv)
		return actedMsg{id: id, item: a.item, action: a.Action, result: res, err: err}
	}
}

// acted toasts an Action's result and, after a success, refreshes its Widget
// and every Widget sharing a file with it, or first hands a file to $EDITOR.
func (m Model) acted(msg actedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		return m.say(capitalise(msg.action.Label)+" failed: "+msg.err.Error(), theme.Bad)
	}
	if msg.byHost {
		return m, nil
	}
	if f := msg.result.Edit; f != "" {
		then, item := msg.result.Then, msg.item
		return m, m.opts.Ports.Editor.Edit(f, func(err error) tea.Msg {
			return editedMsg{id: msg.id, file: f, err: err, then: then, item: item}
		})
	}
	return m.after(msg.id, msg.result.Toast)
}

// edited runs the Action that follows the editor, or refreshes the Widget
// whose file the editor just had, or says why it failed.
func (m Model) edited(msg editedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		return m.say("$EDITOR failed: "+msg.err.Error(), theme.Bad)
	}
	if msg.then != nil {
		return m.invoke(msg.id, bound{Action: *msg.then, item: msg.item}, msg.file)
	}
	return m.after(msg.id, "")
}

// after toasts a change to Widget id and refreshes it and every Widget
// sharing a file with it.
func (m Model) after(id, toast string) (Model, tea.Cmd) {
	var cmds []tea.Cmd
	if toast != "" {
		var cmd tea.Cmd
		m, cmd = m.say(toast, theme.Fg)
		cmds = append(cmds, cmd)
	}
	files := watched(m.widgets[id].widget)
	for _, other := range m.ids() {
		if other == id || slices.ContainsFunc(watched(m.widgets[other].widget), func(f string) bool { return slices.Contains(files, f) }) {
			cmds = append(cmds, m.refresh(other))
		}
	}
	return m, tea.Batch(cmds...)
}

func watched(w widget.Widget) []string {
	if wt, ok := w.(widget.Watcher); ok {
		return wt.Files()
	}
	return nil
}

func capitalise(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[n:]
}
