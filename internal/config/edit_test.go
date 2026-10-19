package config_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/oronbz/visor/internal/config"
	"github.com/oronbz/visor/internal/widget"
)

var editTypes = []widget.Type{
	{Name: "clock", Title: "Clock", Settings: []widget.Setting{{Name: "seconds", Kind: widget.Bool, Default: true}}},
	{Name: "weather", Title: "Weather", Settings: []widget.Setting{
		{Name: "city", Kind: widget.String, Default: "", Required: true},
		{Name: "units", Kind: widget.String, Default: "metric"},
	}},
}

func editorFor(t *testing.T, src string) (config.Editor, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, config.FileName)
	if src != "" {
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return config.Editor{Path: path, LockDir: filepath.Join(dir, "locks"), Types: editTypes}, path
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const handWritten = `# my dashboard
theme = "visor"   # keep this

[[widget]]
type = "weather"   # home
city = "Haifa"
`

func TestAddWidgetAppendsABlockWithAFreshIdAndLeavesTheRestByteIdentical(t *testing.T) {
	e, path := editorFor(t, handWritten)

	w, err := e.AddWidget("weather")
	if err != nil {
		t.Fatal(err)
	}

	want := handWritten + `
[[widget]]
type = "weather"
id = "weather-2"
city = ""
`
	if got := read(t, path); got != want {
		t.Errorf("config after Add Widget:\n%s\nwant:\n%s", got, want)
	}
	if w.ID != "weather-2" || w.Type != "weather" {
		t.Errorf("added Widget %q of type %q, want weather-2 of type weather", w.ID, w.Type)
	}
}

func TestAddWidgetWithNoConfigKeepsTheDefaultWidgetsAndStartsFromTheReference(t *testing.T) {
	e, path := editorFor(t, "")

	if _, err := e.AddWidget("clock"); err != nil {
		t.Fatal(err)
	}

	got := read(t, path)
	if !strings.HasPrefix(got, config.Reference(editTypes, config.Locale{})) {
		t.Errorf("a new config.toml should start with the commented reference:\n%s", got)
	}
	c, err := config.Load(path, editTypes, config.Locale{})
	if err != nil {
		t.Fatal(err)
	}
	if ids := idsOf(c); !slices.Equal(ids, []string{"clock", "weather", "clock-2"}) {
		t.Errorf("Widgets after adding a clock to the default set: %v, want clock weather clock-2", ids)
	}
}

func idsOf(c config.Config) []string {
	var ids []string
	for _, w := range c.Widgets {
		ids = append(ids, w.ID)
	}
	return ids
}

const twoWidgets = `# top comment
[[widget]]
type = "clock"   # the clock
seconds = false

# the weather, below
[[widget]]
type = "weather"
city = "Haifa"
# units = "imperial"

# trailing note
`

func TestRemoveWidgetCommentsOutExactlyItsBlock(t *testing.T) {
	e, path := editorFor(t, twoWidgets)

	if err := e.RemoveWidget(config.Ref{ID: "clock"}); err != nil {
		t.Fatal(err)
	}

	want := `# top comment
# [[widget]]
# type = "clock"   # the clock
# seconds = false

# the weather, below
[[widget]]
type = "weather"
city = "Haifa"
# units = "imperial"

# trailing note
`
	if got := read(t, path); got != want {
		t.Errorf("config after removing clock:\n%s\nwant:\n%s", got, want)
	}
	c, err := config.Load(path, editTypes, config.Locale{})
	if err != nil {
		t.Fatal(err)
	}
	if ids := idsOf(c); !slices.Equal(ids, []string{"weather"}) {
		t.Errorf("Widgets after removing clock: %v, want weather", ids)
	}
}

func TestRemoveWidgetTellsApartWidgetsSharingAnId(t *testing.T) {
	src := "[[widget]]\ntype = \"clock\"\n\n[[widget]]\ntype = \"clock\"\nseconds = false\n"
	e, path := editorFor(t, src)

	if err := e.RemoveWidget(config.Ref{ID: "clock", N: 1}); err != nil {
		t.Fatal(err)
	}

	want := "[[widget]]\ntype = \"clock\"\n\n# [[widget]]\n# type = \"clock\"\n# seconds = false\n"
	if got := read(t, path); got != want {
		t.Errorf("config after removing the second clock:\n%s\nwant:\n%s", got, want)
	}
}

func TestRemovingTheLastWidgetIsRefusedSinceTheDefaultSetWouldReturn(t *testing.T) {
	src := "[[widget]]\ntype = \"clock\"\n"
	e, path := editorFor(t, src)

	if err := e.RemoveWidget(config.Ref{ID: "clock"}); err == nil {
		t.Error("removing the only Widget succeeded")
	}
	if got := read(t, path); got != src {
		t.Errorf("refused edit changed the file:\n%s", got)
	}
}

func TestAnEditThatDoesNotReadBackAsIntendedIsRefusedAndTheFileIsUnchanged(t *testing.T) {
	for name, change := range map[string]func([]byte, config.Config) ([]byte, error){
		"breaks the TOML": func(src []byte, _ config.Config) ([]byte, error) {
			return append(src, "[[widget\n"...), nil
		},
		"changes another Widget": func(src []byte, _ config.Config) ([]byte, error) {
			return []byte(strings.Replace(string(src), "Haifa", "Paris", 1)), nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			e, path := editorFor(t, twoWidgets)
			err := e.Edit(change, func(before, after config.Config) error {
				return config.SameWidgets(before, after)
			})
			if err == nil {
				t.Fatal("edit accepted")
			}
			if got := read(t, path); got != twoWidgets {
				t.Errorf("refused edit changed the file:\n%s", got)
			}
		})
	}
}

func TestAddWidgetIsRefusedWhenTheResultWouldNotParse(t *testing.T) {
	src := "widget = [{ type = \"clock\" }]\n"
	e, path := editorFor(t, src)

	if _, err := e.AddWidget("weather"); err == nil {
		t.Error("appending a [[widget]] block after an inline widget array succeeded")
	}
	if got := read(t, path); got != src {
		t.Errorf("refused edit changed the file:\n%s", got)
	}
}

func TestLocateFindsTheBlockLine(t *testing.T) {
	e, _ := editorFor(t, twoWidgets)

	line, err := e.Locate(config.Ref{ID: "weather"})
	if err != nil {
		t.Fatal(err)
	}
	if line != 7 {
		t.Errorf("weather block at line %d, want 7", line)
	}
}

func TestLocatingADefaultWidgetWritesTheDefaultBlocksFirst(t *testing.T) {
	e, path := editorFor(t, "theme = \"visor\"\n")

	line, err := e.Locate(config.Ref{ID: "weather"})
	if err != nil {
		t.Fatal(err)
	}
	got := read(t, path)
	want := "theme = \"visor\"\n\n[[widget]]\ntype = \"clock\"\nid = \"clock\"\n\n[[widget]]\ntype = \"weather\"\nid = \"weather\"\n"
	if got != want {
		t.Errorf("config after locating a default Widget:\n%s\nwant:\n%s", got, want)
	}
	if line != 7 {
		t.Errorf("weather block at line %d, want 7", line)
	}
}
