package host_test

import (
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/widget"
)

func TestGhudStartsUnfocusedAndTabReturnsToTheLastFocusedWidget(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 100, Height: 21})
	titles := []string{"Clock", "Calendar"}

	if got := focused(t, h, titles); got != "" {
		t.Fatalf("fresh start focuses %q, want nothing", got)
	}
	h.Press("tab")
	if got := focused(t, h, titles); got != "Clock" {
		t.Fatalf("tab on a fresh start focuses %q, want the first Widget", got)
	}
	h.Press("l")
	h.Press("esc")
	if got := focused(t, h, titles); got != "" {
		t.Fatalf("esc leaves %q focused", got)
	}
	h.Press("tab")
	if got := focused(t, h, titles); got != "Calendar" {
		t.Errorf("tab after esc focuses %q, want the last-focused Calendar", got)
	}
}

func TestTabMovesAcrossRowsWrappingToTheHorizontallyNearestWidget(t *testing.T) {
	h := phonetic(t, `{"default": {"rows": [
		{"weight": 1, "widgets": [{"id": "a", "weight": 1}, {"id": "b", "weight": 1}, {"id": "c", "weight": 1}]},
		{"weight": 1, "widgets": [{"id": "d", "weight": 1}, {"id": "e", "weight": 3}]},
		{"weight": 1, "widgets": [{"id": "f", "weight": 1}]}
	]}}`)

	for _, c := range []struct {
		from  string
		press string
		want  string
	}{
		{"1", "tab", "Delta"},
		{"2", "tab", "Echo"},
		{"3", "tab", "Echo"},
		{"4", "shift+tab", "Alpha"},
		{"5", "shift+tab", "Bravo"},
		{"5", "tab", "Foxtrot"},
		{"6", "tab", "Bravo"},
		{"1", "shift+tab", "Foxtrot"},
	} {
		h.Press(c.from, c.press)
		if got := focused(t, h, phoneticTitles); got != c.want {
			t.Errorf("%s from Widget %s focuses %q, want %q", c.press, c.from, got, c.want)
		}
	}
}

var phoneticTitles = []string{"Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot"}

// phonetic is a 96×31 host whose Widget types a–f are titled Alpha–Foxtrot,
// arranged by layoutJSON.
func phonetic(t *testing.T, layoutJSON string) *hosttest.Host {
	t.Helper()
	var types []widget.Type
	for _, title := range phoneticTitles {
		types = append(types, widget.Type{
			Name:  strings.ToLower(title[:1]),
			Title: title,
			New:   func() widget.Widget { return blank{} },
		})
	}
	return hosttest.New(t, hosttest.Options{Width: 96, Height: 31, Root: savedLayout(t, layoutJSON), Types: types})
}

type blank struct{}

func (blank) Payload(widget.Frame) widget.Payload { return widget.Payload{} }

// focused names the one Widget among titles whose border title is bold, or ""
// when none is. It fails the test if more than one is.
func focused(t *testing.T, h *hosttest.Host, titles []string) string {
	t.Helper()
	got := ""
	for _, title := range titles {
		x, y, ok := h.Find("─ " + title + " ")
		if !ok {
			t.Fatalf("title %q not on screen:\n%s", title, h.Screen())
		}
		if h.Bold(x+2, y) {
			if got != "" {
				t.Fatalf("both %q and %q are focused", got, title)
			}
			got = title
		}
	}
	return got
}
