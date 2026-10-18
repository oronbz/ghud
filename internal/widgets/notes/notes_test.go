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

func TestACloseKeyInInsertModeSavesTheEditThenQuits(t *testing.T) {
	for _, keys := range [][]string{{"shift+super+g"}, {"ctrl+b", "d"}, {"ctrl+k", "ctrl+b", "d"}} {
		h := withNotes(t, "Buy milk\n", hosttest.Options{Width: 40, Height: 9, CloseKeys: []string{"cmd+shift+g", "ctrl+b d"}})
		h.Press("1", "i")
		h.Type("Call mum")

		h.Press(keys...)
		if !h.Quit() {
			t.Errorf("%q in INSERT mode did not quit", keys)
		}
		if got := readNote(t, notesFile(h.Root, "notes.md")); got != "Buy milk\nCall mum\n" {
			t.Errorf("%q in INSERT mode left notes.md = %q", keys, got)
		}
	}
}

func TestCtrlKOpensThePaletteFromInsertModeAndColonTypes(t *testing.T) {
	h := withNotes(t, "", hosttest.Options{Width: 60, Height: 16})
	h.Press("1", "i")
	h.Type("a:bc")
	// In the editor alone, ctrl+k would delete to the end of the line.
	h.Press("left", "left", "ctrl+k")
	if !strings.Contains(h.Screen(), "─ Actions ") {
		t.Fatalf("ctrl+k didn't open the palette:\n%s", h.Screen())
	}
	h.Press("esc", "esc")
	if got := readNote(t, notesFile(h.Root, "notes.md")); got != "a:bc\n" {
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

func TestAnInlineSaveOverANoteChangedElsewhereKeepsTheOtherVersionInAConflictFile(t *testing.T) {
	h := withNotes(t, "Buy milk\n", hosttest.Options{Width: 100, Height: 9})
	path := notesFile(h.Root, "notes.md")
	h.Press("1", "i")
	h.Type("Call mum")

	writeNote(t, path, "Buy oat milk\n")
	h.Changed(path)
	h.Press("esc")

	if got := readNote(t, path); got != "Buy milk\nCall mum\n" {
		t.Errorf("notes.md = %q, want the user's save", got)
	}
	conflict := notesFile(h.Root, "notes.conflict-20261018-140509.md")
	if got := readNote(t, conflict); got != "Buy oat milk\n" {
		t.Errorf("conflict file = %q, want the version saved elsewhere", got)
	}
	if _, _, ok := h.Find("other version is in notes.conflict-20261018-140509.md"); !ok {
		t.Errorf("no toast naming the conflict file:\n%s", h.Screen())
	}
}

func TestAnInlineSaveOverAnUnchangedNoteMakesNoConflictFile(t *testing.T) {
	h := withNotes(t, "Buy milk\n", hosttest.Options{Width: 100, Height: 9})
	h.Press("1", "i")
	h.Type("Call mum")
	h.Press("esc")
	entries, err := os.ReadDir(h.Paths.Data)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "conflict") {
			t.Errorf("conflict file %s written for an unchanged note", e.Name())
		}
	}
}

func TestAnEditorSaveOverANoteChangedElsewhereKeepsTheOtherVersionInAConflictFile(t *testing.T) {
	h := withNotes(t, "Buy milk\n", hosttest.Options{Width: 100, Height: 9})
	path := notesFile(h.Root, "notes.md")
	h.Editor.OnEdit = func(p string) error {
		// Another instance saves while the editor is open.
		writeNote(t, path, "Buy oat milk\n")
		return os.WriteFile(p, []byte("Buy milk\nCall mum\n"), 0o644)
	}

	h.Press("1", "E")
	if got := readNote(t, path); got != "Buy milk\nCall mum\n" {
		t.Errorf("notes.md = %q, want the editor's save", got)
	}
	if got := readNote(t, notesFile(h.Root, "notes.conflict-20261018-140509.md")); got != "Buy oat milk\n" {
		t.Errorf("conflict file = %q, want the version saved elsewhere", got)
	}
	if _, _, ok := h.Find("other version is in notes.conflict-20261018-140509.md"); !ok {
		t.Errorf("no toast naming the conflict file:\n%s", h.Screen())
	}
}

func TestQuittingTheEditorWithoutSavingLeavesANoteChangedElsewhere(t *testing.T) {
	h := withNotes(t, "Buy milk\n", hosttest.Options{Width: 100, Height: 9})
	path := notesFile(h.Root, "notes.md")
	h.Editor.OnEdit = func(string) error {
		writeNote(t, path, "Buy oat milk\n")
		return nil
	}

	h.Press("1", "E")
	if got := readNote(t, path); got != "Buy oat milk\n" {
		t.Errorf("notes.md = %q, want the other instance's version untouched", got)
	}
	if _, err := os.Stat(notesFile(h.Root, "notes.conflict-20261018-140509.md")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("conflict file written although the editor saved nothing (stat error %v)", err)
	}
	if got := body(h, 1)[0]; got != "Buy oat milk" {
		t.Errorf("Widget shows %q, want the note as it is on disk", got)
	}
}

func TestEOpensTheNoteInTheEditorAndShowsTheEditedText(t *testing.T) {
	h := withNotes(t, "Buy milk\n", hosttest.Options{Width: 40, Height: 9})
	path := notesFile(h.Root, "notes.md")
	h.Editor.OnEdit = func(p string) error {
		if got := body(h, 1)[0]; got != "Buy milk" {
			t.Errorf("Widget reloaded before the editor returned: %q", got)
		}
		if got := readNote(t, p); got != "Buy milk\n" {
			t.Errorf("the editor got %q, want the note", got)
		}
		return os.WriteFile(p, []byte("Buy oat milk\n"), 0o644)
	}

	h.Press("1", "E")
	// The editor works on a copy named like the note, saved over it on exit.
	if len(h.Editor.Edited) != 1 || filepath.Base(h.Editor.Edited[0]) != "notes.md" {
		t.Fatalf("editor ran on %q, want one notes.md", h.Editor.Edited)
	}
	if got := readNote(t, path); got != "Buy oat milk\n" {
		t.Errorf("notes.md = %q", got)
	}
	if got := body(h, 1)[0]; got != "Buy oat milk" {
		t.Fatalf("Widget after the editor shows %q", got)
	}
	if _, err := os.Stat(h.Editor.Edited[0]); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the editor's copy is still there (stat error %v)", err)
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

func TestTwoNotesWidgetsWithTheirOwnFilesEditIndependently(t *testing.T) {
	root := t.TempDir()
	work, home := notesFile(root, "work.md"), notesFile(root, "home.md")
	writeNote(t, work, "Standup\n")
	writeNote(t, home, "Groceries\n")
	h := hosttest.New(t, hosttest.Options{Width: 60, Height: 9, Root: root, Config: `
[[widget]]
type = "notes"
id = "work"
title = "Work"
file = "work.md"

[[widget]]
type = "notes"
id = "home"
title = "Home"
file = "home.md"
`})
	h.Editor.OnEdit = func(p string) error { return os.WriteFile(p, []byte("Groceries\nGarden\n"), 0o644) }

	h.Press("1", "i")
	h.Type("Ship it")
	h.Press("esc", "2", "E")

	if got := readNote(t, work); got != "Standup\nShip it\n" {
		t.Errorf("work.md = %q", got)
	}
	if got := readNote(t, home); got != "Groceries\nGarden\n" {
		t.Errorf("home.md = %q", got)
	}
	if len(h.Editor.Edited) != 1 || filepath.Base(h.Editor.Edited[0]) != "home.md" {
		t.Errorf("editor ran on %q, want one copy of home.md", h.Editor.Edited)
	}
	if got := strings.Fields(h.Line(2)); strings.Join(got, " ") != "│ Ship it ││ Garden │" {
		t.Errorf("second rows = %q", h.Line(2))
	}
}

func TestTheInlineEditorFollowsATerminalResize(t *testing.T) {
	h := withNotes(t, "", hosttest.Options{Width: 20, Height: 9})
	h.Press("1", "i")
	h.Type("alpha bravo charlie delta")
	h.Resize(40, 9)
	if got := body(h, 1)[0]; got != "alpha bravo charlie delta" {
		t.Fatalf("first row after resize = %q", got)
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
