package host_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/oronbz/visor/internal/config"
	"github.com/oronbz/visor/internal/hosttest"
)

const oneClock = `# my dashboard, by hand
time_format = "24h"   # always

[[widget]]
type = "clock"   # the only one
id = "home"
`

func configText(t *testing.T, h *hosttest.Host) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(h.Paths.Config, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// runFromPalette opens the palette, filters it with query and runs the best match.
func runFromPalette(t *testing.T, h *hosttest.Host, query string) {
	t.Helper()
	h.Press("ctrl+k")
	h.Type(query)
	h.Press("enter")
}

func TestAddWidgetFromThePaletteAppendsABlockAndPlacesTheWidgetLast(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Types: liveViews, Width: 80, Height: 20, Config: oneClock})

	h.Press("ctrl+k")
	h.Type("add calendar")
	if got := paletteEntries(h); len(got) == 0 || !strings.Contains(got[0], "add Calendar Widget") {
		t.Fatalf("with nothing focused, the palette's best match for “add calendar” is %q, want the Add Widget entry", got)
	}
	h.Press("enter")

	want := oneClock + "\n[[widget]]\ntype = \"calendar\"\nid = \"calendar\"\n"
	if got := configText(t, h); got != want {
		t.Errorf("config.toml after Add Widget:\n%s\nwant:\n%s", got, want)
	}
	_, clockY, _ := h.Find("╭─ Clock ")
	_, calY, ok := h.Find("╭─ Calendar ")
	if !ok || calY <= clockY {
		t.Errorf("Calendar not placed in a row below Clock:\n%s", h.Screen())
	}
	if !strings.Contains(h.Screen(), "Added Calendar") {
		t.Errorf("no toast for the added Widget:\n%s", h.Screen())
	}
	noLayoutFile(t, h)
}

func TestRemoveWidgetCommentsOutItsBlockAndTakesItOffTheDashboard(t *testing.T) {
	src := oneClock + "\n[[widget]]\ntype = \"calendar\"\ntitle = \"Month\"\n"
	h := hosttest.New(t, hosttest.Options{Types: liveViews, Width: 80, Height: 20, Config: src})

	runFromPalette(t, h, "month remove")

	want := oneClock + "\n# [[widget]]\n# type = \"calendar\"\n# title = \"Month\"\n"
	if got := configText(t, h); got != want {
		t.Errorf("config.toml after Remove Widget:\n%s\nwant:\n%s", got, want)
	}
	if _, _, ok := h.Find("╭─ Month"); ok {
		t.Errorf("Month still on the dashboard:\n%s", h.Screen())
	}
	if _, _, ok := h.Find("╭─ Clock "); !ok {
		t.Errorf("Clock left the dashboard too:\n%s", h.Screen())
	}
	noLayoutFile(t, h)
}

func TestAConfigEditThatWouldNotReadBackIsRefusedWithAToast(t *testing.T) {
	src := "widget = [{ type = \"clock\" }]\n"
	h := hosttest.New(t, hosttest.Options{Types: liveViews, Width: 80, Height: 20, Config: src})

	runFromPalette(t, h, "add calendar")

	if got := configText(t, h); got != src {
		t.Errorf("refused edit changed config.toml:\n%s", got)
	}
	if !strings.Contains(h.Line(19), "config.toml left unchanged") {
		t.Errorf("footer should say the edit was refused, got %q", h.Line(19))
	}
	if _, _, ok := h.Find("╭─ Calendar"); ok {
		t.Errorf("Calendar placed although the edit was refused:\n%s", h.Screen())
	}
}

func TestEditSettingsOpensTheConfigInTheEditorAtTheWidgetsBlock(t *testing.T) {
	src := oneClock + "\n[[widget]]\ntype = \"calendar\"\n"
	h := hosttest.New(t, hosttest.Options{Types: liveViews, Width: 80, Height: 20, Config: src})

	runFromPalette(t, h, "calendar edit settings")

	path := filepath.Join(h.Paths.Config, config.FileName)
	if !slices.Equal(h.Editor.Edited, []string{path}) || !slices.Equal(h.Editor.Lines, []int{8}) {
		t.Errorf("editor got %v at lines %v, want %s at line 8", h.Editor.Edited, h.Editor.Lines, path)
	}
}

func TestEditSettingsOfADefaultWidgetWritesTheDefaultBlocksFirst(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Types: liveViews, Width: 80, Height: 20})

	runFromPalette(t, h, "calendar edit settings")

	text := configText(t, h)
	c, err := config.Load(filepath.Join(h.Paths.Config, config.FileName), liveViews, config.Locale{})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Widgets) != 2 || c.Widgets[1].ID != "calendar" || len(h.Editor.Lines) != 1 || h.Editor.Lines[0] != c.Widgets[1].Line() {
		t.Errorf("editor opened at %v; config.toml:\n%s", h.Editor.Lines, text)
	}
}
