package host_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/widget"
)

// mailAndTodo stages two list Widgets side by side: Mail with an Item and
// Widget Actions, Todo with one Widget Action. Each records what it ran.
func mailAndTodo(t *testing.T, mail, todo widget.Payload) (h *hosttest.Host, mailRan, todoRan *[]widget.Invocation) {
	t.Helper()
	mailRan, todoRan = &[]widget.Invocation{}, &[]widget.Invocation{}
	typ := func(name, title string, p widget.Payload, ran *[]widget.Invocation) widget.Type {
		return widget.Type{Name: name, Title: title, New: func(widget.Config) widget.Widget {
			return inbox{payload: p, ran: ran}
		}}
	}
	h = hosttest.New(t, hosttest.Options{Width: 100, Height: 24, Types: []widget.Type{
		typ("mail", "Mail", mail, mailRan), typ("todo", "Todo", todo, todoRan),
	}})
	return h, mailRan, todoRan
}

func mailbox() widget.Payload {
	its := items("", "hello", "invoice")
	for i := range its {
		its[i].Actions = []widget.Action{{ID: "archive", Key: "a", Label: "archive"}}
	}
	return widget.Payload{
		Columns: []widget.Column{{Flex: true}},
		Items:   its,
		Actions: []widget.Action{{ID: "compose", Key: "c", Label: "compose"}},
	}
}

func todoList() widget.Payload {
	return widget.Payload{
		Columns: []widget.Column{{Flex: true}},
		Actions: []widget.Action{{ID: "sync", Key: "s", Label: "sync"}},
	}
}

// paletteEntries are the palette's rows as their words, such as "Mail archive a".
func paletteEntries(h *hosttest.Host) []string {
	x, top, ok := h.Find("─ Actions ")
	if !ok {
		return nil
	}
	var out []string
	for _, l := range strings.Split(h.Screen(), "\n")[top+1:] {
		box := []rune(l)[x-1:]
		end := slices.Index(box[1:], '│') + 1
		if box[0] != '│' || end < 1 {
			break
		}
		words := strings.Fields(string(box[1:end]))
		if len(words) > 0 && words[0] != "›" {
			out = append(out, strings.Join(words, " "))
		}
	}
	return out
}

func TestCtrlKOpensAPaletteOfEveryWidgetsActionsAndEnterRunsTheChosenOne(t *testing.T) {
	h, mailRan, todoRan := mailAndTodo(t, mailbox(), todoList())

	h.Press("ctrl+k")
	got := strings.Join(paletteEntries(h), " / ")
	for _, want := range []string{"Mail archive a", "Mail compose c", "Mail hide", "Todo sync s", "Todo hide"} {
		if !strings.Contains(got, want) {
			t.Errorf("palette lacks %q while nothing is focused; it lists %q", want, got)
		}
	}

	h.Press("s", "y", "n")
	if got := paletteEntries(h); len(got) != 1 || got[0] != "Todo sync s" {
		t.Errorf("typing syn leaves %q, want only Todo sync", got)
	}
	h.Press("enter")
	if len(*todoRan) != 1 || (*todoRan)[0].Action != "sync" {
		t.Errorf("Todo ran %+v, want sync", *todoRan)
	}
	if len(*mailRan) != 0 {
		t.Errorf("Mail ran %+v, want nothing", *mailRan)
	}
	if _, _, ok := h.Find("─ Actions "); ok {
		t.Errorf("running an Action left the palette open:\n%s", h.Screen())
	}
}

func TestColonOpensThePaletteToRunTheSelectedItemsActionAndEscClosesIt(t *testing.T) {
	h, mailRan, _ := mailAndTodo(t, mailbox(), todoList())
	h.Press("tab", "j", "esc")

	h.Press(":")
	if _, _, ok := h.Find("─ Actions "); !ok {
		t.Fatalf(": didn't open the palette:\n%s", h.Screen())
	}
	if footer := h.Line(23); !strings.Contains(footer, "⏎ run · esc close") {
		t.Errorf("footer %q, want the palette's keys", footer)
	}
	h.Press("esc")
	if _, _, ok := h.Find("─ Actions "); ok {
		t.Fatalf("esc left the palette open:\n%s", h.Screen())
	}

	h.Press(":", "a", "r", "c")
	if got := paletteEntries(h); len(got) != 1 || got[0] != "Mail archive a" {
		t.Errorf("typing arc leaves %q, want only Mail archive", got)
	}
	h.Press("enter")
	if len(*mailRan) != 1 || (*mailRan)[0].Action != "archive" || (*mailRan)[0].Item != "invoice" {
		t.Errorf("Mail ran %+v, want archive on the selected invoice", *mailRan)
	}
}

func TestThePaletteMovesItsSelectionAndAsksBeforeAConfirmedAction(t *testing.T) {
	todo := todoList()
	todo.Actions = append(todo.Actions, widget.Action{ID: "purge", Key: "p", Label: "purge", Confirm: true})
	h, _, todoRan := mailAndTodo(t, mailbox(), todo)

	h.Press("ctrl+k")
	want := "Mail archive a / Mail compose c / Mail hide / Todo sync s / Todo purge p / Todo hide"
	if got := strings.Join(paletteEntries(h), " / "); got != want {
		t.Fatalf("palette lists %q, want Layout order %q", got, want)
	}
	if x, y, _ := h.Find("archive"); !isSel(h, x, y) {
		t.Errorf("first entry isn't selected:\n%s", h.Screen())
	}
	h.Press("down", "down", "down", "up", "ctrl+n")
	if x, y, _ := h.Find("sync"); !isSel(h, x, y) {
		t.Errorf("down down down up ctrl+n didn't select Todo sync:\n%s", h.Screen())
	}
	h.Press("down")
	if x, y, _ := h.Find("purge"); !isSel(h, x, y) {
		t.Errorf("down didn't select Todo purge:\n%s", h.Screen())
	}
	h.Press("enter")
	if _, _, ok := h.Find("Purge?"); !ok {
		t.Fatalf("a confirmed Action from the palette didn't ask first:\n%s", h.Screen())
	}
	if len(*todoRan) != 0 {
		t.Fatalf("purge ran before it was confirmed: %+v", *todoRan)
	}
	h.Press("y")
	if len(*todoRan) != 1 || (*todoRan)[0].Action != "purge" {
		t.Errorf("Todo ran %+v, want purge after confirming", *todoRan)
	}
}
