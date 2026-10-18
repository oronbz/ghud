package host_test

import (
	"path/filepath"
	"testing"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/layout"
)

func layoutFile(h *hosttest.Host) string { return filepath.Join(h.Paths.Config, layout.FileName) }

func TestAnArrangeChangeInOneInstanceShowsUpInTheOther(t *testing.T) {
	root := savedLayout(t, threeAcross)
	a, b := phoneticAt(t, root), phoneticAt(t, root)

	a.Press("1", "m", "l", "enter")
	b.Changed(layoutFile(b))
	if got := order(t, b, "Alpha", "Bravo", "Charlie"); got != "Bravo Alpha Charlie" {
		t.Fatalf("after a moved Alpha, b's first row reads %q, want Bravo Alpha Charlie:\n%s", got, b.Screen())
	}

	// Last writer wins: b's own change replaces a's.
	b.Press("3", "m", "h", "enter")
	a.Changed(layoutFile(a))
	if got := order(t, a, "Alpha", "Bravo", "Charlie"); got != "Bravo Charlie Alpha" {
		t.Errorf("after b moved Charlie, a's first row reads %q, want Bravo Charlie Alpha:\n%s", got, a.Screen())
	}
}

func TestALayoutSavedElsewhereMidArrangementIsWhatEscRevertsTo(t *testing.T) {
	root := savedLayout(t, threeAcross)
	a, b := phoneticAt(t, root), phoneticAt(t, root)

	b.Press("3", "m", "h")
	a.Press("1", "m", "l", "enter")
	b.Changed(layoutFile(b))
	if _, y, ok := b.Find("┏━ Charlie "); !ok || y != 0 {
		t.Fatalf("the reload ended b's arrangement in progress:\n%s", b.Screen())
	}
	b.Press("esc")
	if got := order(t, b, "Alpha", "Bravo", "Charlie"); got != "Bravo Alpha Charlie" {
		t.Errorf("esc reverted b to %q, want a's saved Bravo Alpha Charlie", got)
	}
}
