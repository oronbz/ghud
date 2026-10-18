package host_test

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/hosttest"
)

func TestTwoClocksWithTheirOwnIdsAndTitlesBothRender(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 100, Height: 12, Config: `
[[widget]]
type = "clock"
id = "home"
title = "Tel Aviv"

[[widget]]
type = "clock"
id = "work"
title = "London"
`})

	if got := cornersOn(h.Line(0)); len(got) != 2 {
		t.Fatalf("want two tiles in one row, got corners at %v:\n%s", got, h.Screen())
	}
	for _, title := range []string{"╭─ Tel Aviv ", "╭─ London "} {
		if _, _, ok := h.Find(title); !ok {
			t.Errorf("%q not on screen:\n%s", title, h.Screen())
		}
	}
	if _, _, ok := h.Find("Calendar"); ok {
		t.Errorf("Calendar shown although the config places only clocks:\n%s", h.Screen())
	}
	if n := strings.Count(h.Screen(), "Sunday 18 October"); n != 2 {
		t.Errorf("date shown %d times, want once per clock:\n%s", n, h.Screen())
	}
}

func TestWidgetIsBuiltFromItsSettings(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 40, Height: 6, Config: `
[[widget]]
type = "clock"
seconds = false
`})

	if !strings.Contains(h.Screen(), "14:05") || strings.Contains(h.Screen(), "14:05:") {
		t.Errorf("clock without seconds, want 14:05:\n%s", h.Screen())
	}
}

func TestTwoWidgetsOfATypeWithoutIdsBothNeedSetup(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 100, Height: 12, Config: `
[[widget]]
type = "clock"

[[widget]]
type = "clock"
title = "Second"
`})

	if n := strings.Count(h.Screen(), "needs setup"); n != 2 {
		t.Errorf("needs setup shown %d times, want on both clocks:\n%s", n, h.Screen())
	}
	if n := strings.Count(h.Screen(), "two `clock` Widgets, add `id`"); n < 2 {
		t.Errorf("hint shown %d times, want on both clocks:\n%s", n, h.Screen())
	}
	if strings.Contains(h.Screen(), "Sunday") {
		t.Errorf("a duplicate clock still draws its date:\n%s", h.Screen())
	}
	if _, _, ok := h.Find("config.toml:2: two `clock` Widgets"); !ok {
		t.Errorf("no toast naming the first block's line:\n%s", h.Screen())
	}
}

func TestWidgetsSharingAnExplicitIdBothNeedSetup(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 100, Height: 12, Config: `
[[widget]]
type = "clock"
id = "desk"

[[widget]]
type = "calendar"
id = "desk"

[[widget]]
type = "clock"
id = "wall"
title = "Wall"
`})

	if n := strings.Count(h.Screen(), "id `desk`"); n != 3 {
		t.Errorf("duplicate-id hint shown %d times, want on both Widgets and the toast:\n%s", n, h.Screen())
	}
	if !strings.Contains(h.Screen(), "Sunday 18 October") {
		t.Errorf("the Wall clock with its own id doesn't render:\n%s", h.Screen())
	}
}

func TestUnknownKeyPutsOnlyThatWidgetInNeedsSetup(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 100, Height: 12, Config: `
[[widget]]
type = "clock"
colour = "red"

[[widget]]
type = "calendar"
`})

	if n := strings.Count(h.Screen(), "needs setup"); n != 1 {
		t.Errorf("needs setup shown %d times, want only on the clock:\n%s", n, h.Screen())
	}
	if !strings.Contains(h.Screen(), "October 2026") {
		t.Errorf("the calendar doesn't render:\n%s", h.Screen())
	}
	if _, _, ok := h.Find("config.toml:4: unknown key `colour` in Widget `clock`"); !ok {
		t.Errorf("no toast naming the key and its line:\n%s", h.Screen())
	}
}

func TestInvalidCommonKeysPutTheirWidgetsInNeedsSetup(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 100, Height: 12, Config: `
[[widget]]
type = "clock"
refresh = "soon"

[[widget]]
type = "calendar"
title = 7
`})

	if n := strings.Count(h.Screen(), "needs setup"); n != 2 {
		t.Errorf("needs setup shown %d times, want on both Widgets:\n%s", n, h.Screen())
	}
	if _, _, ok := h.Find("config.toml:4: `refresh` in Widget `clock` must be a duration"); !ok {
		t.Errorf("no toast for the bad refresh:\n%s", h.Screen())
	}
	if _, _, ok := h.Find("(+1 more)"); !ok {
		t.Errorf("toast doesn't count the second problem:\n%s", h.Screen())
	}
}

func TestUnknownWidgetTypeNeedsSetupAndOthersRender(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 100, Height: 12, Config: `
[[widget]]
type = "sundial"

[[widget]]
type = "clock"
`})

	if _, _, ok := h.Find("unknown Widget type `sundial`"); !ok {
		t.Errorf("no hint for the unknown type:\n%s", h.Screen())
	}
	if _, _, ok := h.Find("╭─ sundial "); !ok {
		t.Errorf("the unknown type isn't titled by its name:\n%s", h.Screen())
	}
	if !strings.Contains(h.Screen(), "Sun 18 Oct") {
		t.Errorf("the clock doesn't render:\n%s", h.Screen())
	}
}

func TestSyntaxErrorAtLaunchStartsOnTheDefaultWithAToast(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 100, Height: 16, Types: liveViews, Config: `
[[widget]]
type = "clock"
title = "Broken
`})

	if got, want := h.Line(0), "╭─ Clock ────────────────────────────────────────╮╭─ Calendar ─────────────────────────────────────╮"; got != want {
		t.Errorf("not the default Layout:\n%s\nwant:\n%s", got, want)
	}
	if _, _, ok := h.Find("config.toml:4: "); !ok {
		t.Errorf("no toast naming file:line:\n%s", h.Screen())
	}
}

func TestBadGlobalOptionToastsAndKeepsTheDefault(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 100, Height: 16, Config: `
time_format = "25h"
`})

	if _, _, ok := h.Find("config.toml:2: `time_format` must be one of \"12h\", \"24h\""); !ok {
		t.Errorf("no toast for the bad option:\n%s", h.Screen())
	}
	if _, _, ok := h.Find("╭─ Calendar "); !ok {
		t.Errorf("a config with no [[widget]] blocks doesn't show the default Widgets:\n%s", h.Screen())
	}
}

func TestMisshapenWidgetAndPluginEntriesToastWithoutCrashing(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 100, Height: 16, Config: `
widget = [1, 2]
plugin = ["x"]
`})

	if _, _, ok := h.Find("config.toml:2: [[widget]] needs a `type` (+2 more)"); !ok {
		t.Errorf("no toast for the misshapen entries:\n%s", h.Screen())
	}
	if _, _, ok := h.Find("╭─ Clock "); !ok {
		t.Errorf("the default Widgets don't render:\n%s", h.Screen())
	}
}

func TestLaunchingWithNoConfigWritesNoFiles(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{})
	h.Press("j", "q")

	err := filepath.WalkDir(h.Root, func(path string, d fs.DirEntry, err error) error {
		if err == nil && path == filepath.Join(h.Root, "cache") {
			return fs.SkipDir
		}
		if err == nil && path == filepath.Join(h.Root, "cache") {
			return fs.SkipDir
		}
		if err == nil && path != h.Root {
			t.Errorf("wrote %s", path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := h.Find("config.toml"); ok {
		t.Errorf("a missing config toasts:\n%s", h.Screen())
	}
}
