package notes_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/paths"
)

const oneNotesWidget = `
[[widget]]
type = "notes"
`

// withNotes is a host showing one Notes Widget over notes.md in the data
// dir, which holds md before the app starts.
func withNotes(t *testing.T, md string, o hosttest.Options) *hosttest.Host {
	t.Helper()
	if o.Root == "" {
		o.Root = t.TempDir()
	}
	writeNote(t, notesFile(o.Root, "notes.md"), md)
	if o.Config == "" {
		o.Config = oneNotesWidget
	}
	return hosttest.New(t, o)
}

func notesFile(root, name string) string { return filepath.Join(paths.Under(root).Data, name) }

func writeNote(t *testing.T, path, md string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readNote(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// body is the text inside the first tile's border, one entry per row,
// margins and scroll thumb trimmed.
func body(h *hosttest.Host, rows int) []string {
	lines := strings.Split(h.Screen(), "\n")
	var out []string
	for _, l := range lines[1 : 1+rows] {
		out = append(out, strings.Trim(l, "│▐ "))
	}
	return out
}

func TestNotesShowsTheNoteAsWrappedText(t *testing.T) {
	h := withNotes(t, "# Today\nShip the notes widget with wrapping that goes past the edge\n", hosttest.Options{Width: 30, Height: 7})

	if got := h.Line(0); got != "╭─ Notes ────────────────────╮" {
		t.Fatalf("top border = %q", got)
	}
	want := []string{"# Today", "Ship the notes widget with", "wrapping that goes past", "the edge"}
	if got := body(h, 4); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("body =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestJAndKScrollTheFocusedNote(t *testing.T) {
	h := withNotes(t, "one\ntwo\nthree\nfour\nfive\nsix\nseven\n", hosttest.Options{Width: 30, Height: 7})
	shown := func() string { return strings.Join(body(h, 4), " ") }

	h.Press("1", "j")
	if got := shown(); got != "two three four five" {
		t.Fatalf("after j: %q", got)
	}
	h.Press("G")
	if got := shown(); got != "four five six seven" {
		t.Fatalf("after G: %q", got)
	}
	h.Press("j")
	if got := shown(); got != "four five six seven" {
		t.Fatalf("j past the end scrolled: %q", got)
	}
	h.Press("k", "g", "k")
	if got := shown(); got != "one two three four" {
		t.Fatalf("after k g k: %q", got)
	}
}
