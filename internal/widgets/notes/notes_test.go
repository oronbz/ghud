package notes_test

import (
	"errors"
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

func TestIEditsTheNoteInInsertModeAndEscSavesIt(t *testing.T) {
	h := withNotes(t, "Buy milk\n", hosttest.Options{Width: 40, Height: 9})
	h.Press("1", "i")
	if got := h.Line(8); !strings.HasPrefix(got, " INSERT ") {
		t.Fatalf("footer while editing = %q", got)
	}
	h.Type("Call mum, quick?")
	if got := body(h, 2); got[1] != "Call mum, quick?" {
		t.Fatalf("editor shows %q", got)
	}
	if b := readNote(t, notesFile(h.Root, "notes.md")); b != "Buy milk\n" {
		t.Fatalf("saved before esc: %q", b)
	}

	h.Press("esc")
	if got := readNote(t, notesFile(h.Root, "notes.md")); got != "Buy milk\nCall mum, quick?\n" {
		t.Fatalf("notes.md = %q", got)
	}
	if got := h.Line(8); !strings.HasPrefix(got, " NORMAL ") {
		t.Fatalf("footer after esc = %q", got)
	}
	if got := body(h, 2); got[0] != "Buy milk" || got[1] != "Call mum, quick?" {
		t.Fatalf("Widget after save shows %q", got)
	}
}

func TestCtrlKStaysAHostKeyInInsertMode(t *testing.T) {
	h := withNotes(t, "", hosttest.Options{Width: 40, Height: 9})
	h.Press("1", "i")
	h.Type("abc")
	// In the editor alone, ctrl+k would delete to the end of the line.
	h.Press("left", "left", "ctrl+k", "esc")
	if got := readNote(t, notesFile(h.Root, "notes.md")); got != "abc\n" {
		t.Fatalf("notes.md = %q", got)
	}
}

func TestLeavingInsertModeWithoutChangesWritesNothing(t *testing.T) {
	h := withNotes(t, "Buy milk", hosttest.Options{Width: 40, Height: 9})
	h.Press("1", "i", "left", "esc")
	if got := readNote(t, notesFile(h.Root, "notes.md")); got != "Buy milk" {
		t.Fatalf("notes.md = %q", got)
	}
	if got := h.Line(8); strings.Contains(got, "Saved") {
		t.Fatalf("footer = %q", got)
	}
}

func TestEOpensTheNoteInTheEditorAndShowsTheEditedText(t *testing.T) {
	h := withNotes(t, "Buy milk\n", hosttest.Options{Width: 40, Height: 9})
	path := notesFile(h.Root, "notes.md")
	h.Editor.OnEdit = func(p string) error {
		if got := body(h, 1)[0]; got != "Buy milk" {
			t.Errorf("Widget reloaded before the editor returned: %q", got)
		}
		return os.WriteFile(p, []byte("Buy oat milk\n"), 0o644)
	}

	h.Press("1", "E")
	if len(h.Editor.Edited) != 1 || h.Editor.Edited[0] != path {
		t.Fatalf("editor ran on %q, want [%s]", h.Editor.Edited, path)
	}
	if got := body(h, 1)[0]; got != "Buy oat milk" {
		t.Fatalf("Widget after the editor shows %q", got)
	}
}

func TestAFailedEditorRunIsToasted(t *testing.T) {
	h := withNotes(t, "Buy milk\n", hosttest.Options{Width: 60, Height: 9})
	h.Editor.OnEdit = func(string) error { return errors.New("exit status 1") }
	h.Press("1", "E")
	if got := h.Line(8); !strings.Contains(got, "$EDITOR failed: exit status 1") {
		t.Fatalf("footer = %q", got)
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
