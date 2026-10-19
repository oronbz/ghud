package host_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/oronbz/visor/internal/hosttest"
	"github.com/oronbz/visor/internal/layout"
)

func TestWHidesAWidgetReflowsTheGridAndRecordsItHidden(t *testing.T) {
	root := savedLayout(t, threeAcross)
	h := phoneticAt(t, root)

	h.Press("w")
	for _, line := range []string{"[x] Alpha", "[x] Bravo", "[x] Charlie", "[x] Delta"} {
		if _, _, ok := h.Find(line); !ok {
			t.Fatalf("w list lacks %q:\n%s", line, h.Screen())
		}
	}
	if footer := h.Line(30); !strings.Contains(footer, "␣ show/hide") {
		t.Errorf("footer %q, want the list's keys", footer)
	}
	h.Press("j", "space")
	if _, _, ok := h.Find("[ ] Bravo"); !ok {
		t.Errorf("space left Bravo shown in the list:\n%s", h.Screen())
	}
	h.Press("esc")
	if _, _, ok := h.Find("Widgets"); ok {
		t.Fatalf("esc left the list open:\n%s", h.Screen())
	}
	if _, _, ok := h.Find("─ Bravo "); ok {
		t.Errorf("hidden Bravo still drawn:\n%s", h.Screen())
	}
	if got := cornersOn(h.Line(0)); !slices.Equal(got, []int{0, 48}) {
		t.Errorf("top row has tiles at %v, want Alpha and Charlie sharing it at [0 48]:\n%s", got, h.Screen())
	}
	saved, err := layout.Load(h.Paths.Config)
	if err != nil {
		t.Fatalf("layout.json not written: %v", err)
	}
	if !slices.Equal(saved.Hidden, []string{"b"}) {
		t.Errorf("saved hidden %v, want [b]", saved.Hidden)
	}

	restarted := phoneticAt(t, root)
	if _, _, ok := restarted.Find("─ Bravo "); ok {
		t.Errorf("Bravo drawn again after a restart:\n%s", restarted.Screen())
	}
	restarted.Press("w", "j", "enter", "w")
	if got := order(t, restarted, "Alpha", "Bravo", "Charlie"); got != "Alpha Bravo Charlie" {
		t.Errorf("showing Bravo again gives a top row of %q, want it back in its place", got)
	}
}

func TestHidingTheFocusedWidgetUnfocusesIt(t *testing.T) {
	h := phonetic(t, threeAcross)
	h.Press("2", "w", "j", "space", "esc")
	if got := focused(t, h, []string{"Alpha", "Charlie", "Delta"}); got != "" {
		t.Errorf("hiding focused Bravo moved focus to %q, want visor unfocused", got)
	}
	h.Press("tab")
	if got := focused(t, h, []string{"Alpha", "Charlie", "Delta"}); got != "Alpha" {
		t.Errorf("tab after hiding the last-focused Widget focuses %q, want the first visible one", got)
	}
}

func TestHidingEveryWidgetShowsAHintAndSurvivesARestart(t *testing.T) {
	root := savedLayout(t, threeAcross)
	h := phoneticAt(t, root)

	h.Press("w")
	for range phoneticTitles {
		h.Press("space", "j")
	}
	h.Press("esc")
	if _, _, ok := h.Find("all Widgets hidden · w to show"); !ok {
		t.Fatalf("no hint once every Widget is hidden:\n%s", h.Screen())
	}
	h.Press("tab", "h", "1", "m", "H", "?", "?")

	restarted := phoneticAt(t, root)
	restarted.Press("tab", "j", "l")
	if _, _, ok := restarted.Find("all Widgets hidden · w to show"); !ok {
		t.Fatalf("a restart over an all-hidden layout.json lacks the hint:\n%s", restarted.Screen())
	}
	restarted.Press("w", "enter", "esc")
	if _, _, ok := restarted.Find("─ Alpha "); !ok {
		t.Errorf("showing Alpha again doesn't draw it:\n%s", restarted.Screen())
	}
}

func TestTheWidgetListSwallowsOtherKeys(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 100, Height: 21})
	h.Press("w", "tab", "m", "k", "k", "q")
	if h.Quit() {
		t.Fatal("q in the Widget list quit visor")
	}
	if got := focused(t, h, []string{"Clock", "Calendar"}); got != "" {
		t.Errorf("keys in the Widget list focused %q", got)
	}
	noLayoutFile(t, h)
}
