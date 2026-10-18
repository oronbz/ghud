package host_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

// inbox is a list Widget type serving a fixed Payload and recording every
// Action it's asked to run.
type inbox struct {
	payload widget.Payload
	ran     *[]widget.Invocation
	err     error
}

func (i inbox) Refresh(context.Context, time.Time) (widget.Payload, error) { return i.payload, nil }

func (i inbox) Invoke(_ context.Context, inv widget.Invocation) (widget.Result, error) {
	*i.ran = append(*i.ran, inv)
	return widget.Result{}, i.err
}

func withInbox(t *testing.T, height int, p widget.Payload, err error) (*hosttest.Host, *[]widget.Invocation) {
	t.Helper()
	ran := &[]widget.Invocation{}
	typ := widget.Type{Name: "inbox", Title: "Inbox", New: func(widget.Config) widget.Widget {
		return inbox{payload: p, ran: ran, err: err}
	}}
	return hosttest.New(t, hosttest.Options{Width: 40, Height: height, Types: []widget.Type{typ}}), ran
}

func items(group string, titles ...string) []widget.Item {
	var its []widget.Item
	for _, s := range titles {
		its = append(its, widget.Item{ID: s, Group: group, Fields: []widget.Field{widget.F(theme.Fg, s)}})
	}
	return its
}

func TestGroupHeadersDivideTheListAndSelectionSkipsThem(t *testing.T) {
	p := widget.Payload{
		Columns: []widget.Column{{Flex: true}},
		Items:   append(items("", "alpha", "bravo"), items("Handled by a teammate", "charlie", "delta", "echo")...),
		Count:   2,
	}
	h, _ := withInbox(t, 6, p, nil)
	shown := func() string {
		var rows []string
		for _, l := range strings.Split(h.Screen(), "\n")[1:4] {
			rows = append(rows, strings.Trim(l, "│▐ ─"))
		}
		return strings.Join(rows, " / ")
	}

	if got, want := h.Line(3), "│ Handled by a teammate ────────────── │"; got != want {
		t.Errorf("group header %q, want %q", got, want)
	}
	if _, _, ok := h.Find("Inbox 2"); !ok {
		t.Errorf("title lacks the Widget's own count:\n%s", h.Screen())
	}

	h.Press("tab", "j", "j")
	if got, want := shown(), "bravo / Handled by a teammate / charlie"; got != want {
		t.Errorf("j from bravo shows %q, want %q", got, want)
	}
	if x, y, _ := h.Find("charlie"); !isSel(h, x, y) {
		t.Errorf("j from bravo didn't select charlie past the header:\n%s", h.Screen())
	}

	h.Press("G")
	if got, want := shown(), "charlie / delta / echo"; got != want {
		t.Errorf("G shows %q, want %q", got, want)
	}
	h.Press("k", "k")
	if got, want := shown(), "Handled by a teammate / charlie / delta"; got != want {
		t.Errorf("moving up to the group's first Item shows %q, want its header with it, %q", got, want)
	}
}

func TestHostKeysWinOverActionKeysAndEnterRunsThePrimaryAction(t *testing.T) {
	its := items("", "alpha", "bravo")
	for i := range its {
		its[i].Actions = []widget.Action{
			{ID: "jump", Key: "j", Label: "jump"},
			{ID: "explode", Key: "x", Label: "explode"},
			{ID: "open", Label: "open", Primary: true, Verb: widget.Open},
		}
	}
	h, ran := withInbox(t, 6, widget.Payload{Columns: []widget.Column{{Flex: true}}, Items: its}, nil)
	h.Press("tab")

	if got := h.Line(4); !strings.Contains(got, "x explode") || !strings.Contains(got, "⏎ open") || strings.Contains(got, "jump") {
		t.Errorf("bottom border %q, want x explode and ⏎ open but not the shadowed j", got)
	}
	h.Press("j", "x", "enter")
	want := []widget.Invocation{{Action: "explode", Item: "bravo"}, {Action: "open", Item: "bravo"}}
	if len(*ran) != len(want) {
		t.Fatalf("ran %+v, want %+v", *ran, want)
	}
	for i, inv := range *ran {
		if inv.Action != want[i].Action || inv.Item != want[i].Item {
			t.Errorf("run %d: %s on %q, want %s on %q", i, inv.Action, inv.Item, want[i].Action, want[i].Item)
		}
	}
}

func TestAFailedActionToastsItsErrorInTheBadColour(t *testing.T) {
	p := widget.Payload{
		Columns: []widget.Column{{Flex: true}},
		Actions: []widget.Action{{ID: "sync", Key: "s", Label: "sync"}},
	}
	h, _ := withInbox(t, 6, p, errors.New("offline"))
	h.Press("tab", "s")

	x, y, ok := h.Find("Sync failed: offline")
	if !ok {
		t.Fatalf("no failure toast:\n%s", h.Screen())
	}
	if fg, ok := h.Fg(x, y); !ok || fg != theme.Bad {
		t.Errorf("failure toast drawn in %v (theme colour %v), want bad", fg, ok)
	}
}

func isSel(h *hosttest.Host, x, y int) bool {
	bg, ok := h.Bg(x, y)
	return ok && bg == theme.Sel
}
