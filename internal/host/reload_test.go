package host_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oronbz/ghud/internal/config"
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

const homeAndWork = `
[[widget]]
type = "clock"
id = "home"
title = "Tel Aviv"

[[widget]]
type = "clock"
id = "work"
title = "London"
`

func configFile(h *hosttest.Host) string { return filepath.Join(h.Paths.Config, config.FileName) }

func writeConfig(t *testing.T, h *hosttest.Host, toml string) {
	t.Helper()
	if err := os.WriteFile(configFile(h), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	h.Changed(configFile(h))
}

func TestBreakingTheConfigWhileRunningKeepsTheWidgetsWithAStickyToastUntilItParses(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 100, Height: 12, Config: homeAndWork})

	writeConfig(t, h, "[[widget]]\ntype = \"clock\"\ntitle = \n")
	for _, title := range []string{"╭─ Tel Aviv ", "╭─ London "} {
		if _, _, ok := h.Find(title); !ok {
			t.Errorf("%q gone after the config broke:\n%s", title, h.Screen())
		}
	}
	if got := h.Line(11); !strings.Contains(got, "config.toml:3:") {
		t.Fatalf("footer %q, want the file:line of the break", got)
	}

	h.Advance(time.Minute)
	if got := h.Line(11); !strings.Contains(got, "config.toml:3:") {
		t.Fatalf("the toast went away while the config is still broken: %q", got)
	}

	writeConfig(t, h, "[[widget]]\ntype = \"clock\"\ntitle = \"Home\"\n")
	if got := h.Line(11); strings.Contains(got, "config.toml") {
		t.Errorf("toast still shown once the config parses: %q", got)
	}
	if _, _, ok := h.Find("╭─ Home "); !ok {
		t.Errorf("the fixed config isn't applied:\n%s", h.Screen())
	}
	if _, _, ok := h.Find("London"); ok {
		t.Errorf("a Widget the fixed config dropped is still shown:\n%s", h.Screen())
	}
}

func TestALiveConfigEditKeepsAnUnchangedWidgetAsItWas(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 100, Height: 12, Config: `
[[widget]]
type = "calendar"

[[widget]]
type = "clock"
`})
	h.Press("2", "n")
	if _, _, ok := h.Find("November 2026"); !ok {
		t.Fatalf("n didn't show next month:\n%s", h.Screen())
	}

	writeConfig(t, h, `
[[widget]]
type = "calendar"

[[widget]]
type = "clock"
title = "Time"
`)
	if _, _, ok := h.Find("╭─ Time "); !ok {
		t.Fatalf("the clock's new title isn't shown:\n%s", h.Screen())
	}
	if _, _, ok := h.Find("November 2026"); !ok {
		t.Errorf("the untouched calendar was rebuilt and lost its month:\n%s", h.Screen())
	}
	if got := focused(t, h, []string{"Calendar", "Time"}); got != "Calendar" {
		t.Errorf("focus moved to %q, want Calendar still", got)
	}
}

func TestALiveConfigEditAddingAWidgetShowsIt(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 60, Height: 11, Config: "[[widget]]\ntype = \"clock\"\n"})
	writeConfig(t, h, "[[widget]]\ntype = \"clock\"\n\n[[widget]]\ntype = \"clock\"\nid = \"work\"\ntitle = \"Work\"\nseconds = false\n")
	if _, _, ok := h.Find("╭─ Work "); !ok {
		t.Fatalf("the added Widget isn't shown:\n%s", h.Screen())
	}
	h.Advance(time.Second)
	if n := strings.Count(h.Screen(), "14:05:10"); n != 1 {
		t.Errorf("14:05:10 shown %d times a second later, want once (the first clock ticking):\n%s", n, h.Screen())
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
