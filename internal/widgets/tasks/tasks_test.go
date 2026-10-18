package tasks_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/paths"
	"github.com/oronbz/ghud/internal/theme"
)

const oneTasksWidget = `
[[widget]]
type = "tasks"
`

// withTasks is a host showing one Tasks Widget over tasks.md in the data dir,
// which holds md before the app starts.
func withTasks(t *testing.T, md string, o hosttest.Options) *hosttest.Host {
	t.Helper()
	if o.Root == "" {
		o.Root = t.TempDir()
	}
	writeTasks(t, o.Root, md)
	if o.Config == "" {
		o.Config = oneTasksWidget
	}
	return hosttest.New(t, o)
}

func tasksFile(root string) string { return filepath.Join(paths.Under(root).Data, "tasks.md") }

func writeTasks(t *testing.T, root, md string) {
	t.Helper()
	if err := os.MkdirAll(paths.Under(root).Data, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tasksFile(root), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readTasks(t *testing.T, h *hosttest.Host) string {
	t.Helper()
	b, err := os.ReadFile(tasksFile(h.Root))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// tile is the screen above the footer.
func tile(h *hosttest.Host) string {
	lines := strings.Split(h.Screen(), "\n")
	return strings.Join(lines[:len(lines)-1], "\n")
}

const sample = `# Tasks

- [ ] Renew passport due:2026-10-20
- [x] Collect Ghostty notes
- [ ] Call the bank
- [ ] Book dentist due:2026-10-18
`

func TestTasksShowsOpenTasksWithDueDatesAndHidesDoneOnes(t *testing.T) {
	h := withTasks(t, sample, hosttest.Options{Width: 40, Height: 7})

	want := strings.Join([]string{
		"╭─ Tasks 3 ────────────────────────────╮",
		"│ ○ Renew passport                 Tue │",
		"│ ○ Call the bank                      │",
		"│ ○ Book dentist                 today │",
		"│                                      │",
		"╰──────────────────────────────────────╯",
	}, "\n")
	if got := tile(h); got != want {
		t.Errorf("tasks tile:\n%s\nwant:\n%s", got, want)
	}
}

func TestNarrowingTheWidgetDropsTheDueColumnBeforeTheTitle(t *testing.T) {
	h := withTasks(t, sample, hosttest.Options{Width: 30, Height: 7})
	if got, want := h.Line(1), "│ ○ Renew passport       Tue │"; got != want {
		t.Fatalf("at width 30: %q, want %q", got, want)
	}

	h.Resize(18, 7)
	if got, want := h.Line(1), "│ ○ Renew passp… │"; got != want {
		t.Errorf("at width 18: %q, want the title kept and the due date dropped, %q", got, want)
	}
	if got, want := h.Line(3), "│ ○ Book dentist │"; got != want {
		t.Errorf("at width 18: %q, want %q", got, want)
	}
}

func TestTheFocusedTasksWidgetHighlightsTheSelectionAndShowsItsKeys(t *testing.T) {
	h := withTasks(t, sample, hosttest.Options{Width: 60, Height: 7})
	if got := h.Line(5); strings.Contains(got, "add") {
		t.Fatalf("unfocused bottom border shows keys: %q", got)
	}

	h.Press("tab")
	if got, want := h.Line(5), "╰─ ␣ done ─ ⏎ edit ─ d delete ─ a add ─────────────────────╯"; got != want {
		t.Errorf("focused bottom border %q, want %q", got, want)
	}
	x, y, _ := h.Find("Renew passport")
	if bg, ok := h.Bg(x, y); !ok || bg != theme.Sel {
		t.Errorf("selected task background %v (theme colour %v), want sel", bg, ok)
	}
	if fg, ok := h.Fg(x, y); !ok || fg != theme.Bright || !h.Bold(x, y) {
		t.Errorf("selected task drawn in %v (theme colour %v, bold %v), want bold bright", fg, ok, h.Bold(x, y))
	}
	x, y, _ = h.Find("Call the bank")
	if bg, ok := h.Bg(x, y); ok && bg == theme.Sel {
		t.Errorf("unselected task is highlighted")
	}

	for _, c := range []struct {
		keys []string
		want string
	}{
		{[]string{"j"}, "Call the bank"},
		{[]string{"down"}, "Book dentist"},
		{[]string{"j"}, "Book dentist"},
		{[]string{"k"}, "Call the bank"},
		{[]string{"g"}, "Renew passport"},
		{[]string{"up"}, "Renew passport"},
		{[]string{"G"}, "Book dentist"},
	} {
		h.Press(c.keys...)
		if got := selected(t, h); got != c.want {
			t.Errorf("%v selects %q, want %q", c.keys, got, c.want)
		}
	}
}

func TestAAddsATaskThroughTheHostPromptWithARelativeDueDate(t *testing.T) {
	h := withTasks(t, sample, hosttest.Options{Width: 80, Height: 12})
	h.Press("tab", "a")
	if _, _, ok := h.Find("New task (end with “due tue” to set a date)"); !ok {
		t.Fatalf("a opened no prompt:\n%s", h.Screen())
	}
	if footer := h.Line(11); !strings.HasPrefix(footer, " INPUT ") {
		t.Errorf("footer %q, want the INPUT badge while the prompt is open", footer)
	}

	typeText(h, "Call bank quickly due tue")
	if h.Quit() {
		t.Fatal("typing q into the prompt quit ghud")
	}
	if _, _, ok := h.Find("› Call bank quickly due tue"); !ok {
		t.Errorf("prompt doesn't echo the typed text:\n%s", h.Screen())
	}
	h.Press("enter")

	if got, want := readTasks(t, h), sample+"- [ ] Call bank quickly due:2026-10-20\n"; got != want {
		t.Errorf("tasks.md after adding:\n%s\nwant:\n%s", got, want)
	}
	if _, _, ok := h.Find("New task"); ok {
		t.Errorf("prompt still open after enter:\n%s", h.Screen())
	}
	if footer := h.Line(11); !strings.Contains(footer, "Added “Call bank quickly”") {
		t.Errorf("footer %q, want a toast confirming the add", footer)
	}
	if _, _, ok := h.Find("○ Call bank quickly"); !ok {
		t.Errorf("the new task isn't listed:\n%s", h.Screen())
	}
	if _, _, ok := h.Find("Tasks 4"); !ok {
		t.Errorf("the count didn't go up:\n%s", h.Screen())
	}

	h.Advance(5 * time.Second)
	if footer := h.Line(11); strings.Contains(footer, "Added") {
		t.Errorf("footer %q still shows the toast after 5s", footer)
	}
}

func TestEscCancelsThePromptWithoutWriting(t *testing.T) {
	h := withTasks(t, sample, hosttest.Options{Width: 80, Height: 12})
	h.Press("tab", "a")
	typeText(h, "Never mind")
	h.Press("esc")

	if got := readTasks(t, h); got != sample {
		t.Errorf("tasks.md changed after esc:\n%s", got)
	}
	if _, _, ok := h.Find("New task"); ok {
		t.Errorf("prompt still open after esc:\n%s", h.Screen())
	}
	if got := selected(t, h); got != "Renew passport" {
		t.Errorf("esc on the prompt left %q selected; want the Widget still focused on its first task", got)
	}
}

func TestEditDoneAndDeleteChangeOnlyTheirOwnLine(t *testing.T) {
	src := "# Tasks\r\n\nnotes here  \n- [ ] Renew passport due:2026-10-20\n  - [ ] Call the bank\n- [x] Old\n* [ ] Book dentist"
	h := withTasks(t, src, hosttest.Options{Width: 80, Height: 12})
	h.Press("tab", "j", "enter")
	if _, _, ok := h.Find("› Call the bank"); !ok {
		t.Fatalf("enter opened no edit prompt holding the task:\n%s", h.Screen())
	}
	h.Press("backspace", "backspace", "backspace", "backspace")
	typeText(h, "office due 2026-10-25")
	h.Press("enter")

	want := "# Tasks\r\n\nnotes here  \n- [ ] Renew passport due:2026-10-20\n  - [ ] Call the office due:2026-10-25\n- [x] Old\n* [ ] Book dentist"
	if got := readTasks(t, h); got != want {
		t.Fatalf("tasks.md after the edit:\n%q\nwant:\n%q", got, want)
	}
	if got := selected(t, h); got != "Call the office" {
		t.Errorf("after the edit %q is selected, want the edited task", got)
	}
	if !strings.Contains(h.Line(11), "Saved “Call the office”") {
		t.Errorf("footer %q, want a toast confirming the edit", h.Line(11))
	}

	h.Press("space")
	want = strings.Replace(want, "  - [ ] Call the office", "  - [x] Call the office", 1)
	if got := readTasks(t, h); got != want {
		t.Fatalf("tasks.md after done:\n%q\nwant:\n%q", got, want)
	}
	if strings.Contains(tile(h), "Call the office") {
		t.Errorf("a done task is still listed:\n%s", h.Screen())
	}
	if got := selected(t, h); got != "Book dentist" {
		t.Errorf("after done %q is selected, want the next task", got)
	}
	if !strings.Contains(h.Line(11), "Done “Call the office”") {
		t.Errorf("footer %q, want a toast confirming done", h.Line(11))
	}

	h.Press("d")
	if _, _, ok := h.Find("Delete?"); !ok {
		t.Fatalf("d asked nothing:\n%s", h.Screen())
	}
	h.Press("esc")
	if got := readTasks(t, h); got != want {
		t.Fatalf("tasks.md changed after cancelling the delete:\n%q", got)
	}
	h.Press("d", "enter")
	want = strings.TrimSuffix(want, "* [ ] Book dentist")
	if got := readTasks(t, h); got != want {
		t.Errorf("tasks.md after delete:\n%q\nwant:\n%q", got, want)
	}
	if !strings.Contains(h.Line(11), "Deleted “Book dentist”") {
		t.Errorf("footer %q, want a toast confirming the delete", h.Line(11))
	}
	if got := selected(t, h); got != "Renew passport" {
		t.Errorf("after deleting the last task %q is selected, want the one above", got)
	}
}

func TestTwoWidgetsOnOneFileShareItAndKeepTheirSelectionAcrossAReload(t *testing.T) {
	h := withTasks(t, sample, hosttest.Options{Width: 100, Height: 10, Config: `
[[widget]]
type = "tasks"
id = "home"
title = "Home"

[[widget]]
type = "tasks"
id = "errands"
title = "Errands"
`})
	h.Press("tab", "G")
	if got := selected(t, h); got != "Book dentist" {
		t.Fatalf("G selects %q, want the last task", got)
	}

	writeTasks(t, h.Root, "- [ ] Water plants\n- [ ] Pay rent\n"+sample)
	h.Press("l", "a")
	typeText(h, "Buy milk")
	h.Press("enter", "h")

	if got := selected(t, h); got != "Book dentist" {
		t.Errorf("after the file reloaded %q is selected in Home, want Book dentist still", got)
	}
	if n := strings.Count(tile(h), "Buy milk"); n != 2 {
		t.Errorf("the task added in Errands shows %d times, want in both Widgets:\n%s", n, h.Screen())
	}
	if n := strings.Count(tile(h), "Water plants"); n != 2 {
		t.Errorf("the externally added task shows %d times, want in both Widgets:\n%s", n, h.Screen())
	}
}

func TestScrollingPastTheEndThenUpMovesAtOnce(t *testing.T) {
	var md strings.Builder
	for _, s := range []string{"one", "two", "three", "four", "five", "six", "seven", "eight"} {
		md.WriteString("- [ ] " + s + "\n")
	}
	h := withTasks(t, md.String(), hosttest.Options{Width: 30, Height: 8})
	visible := func() string {
		var titles []string
		lines := strings.Split(tile(h), "\n")
		for _, l := range lines[1 : len(lines)-1] {
			titles = append(titles, strings.Fields(strings.Trim(l, "│▐ ○"))[0])
		}
		return strings.Join(titles, " ")
	}

	h.Press("tab")
	for range 20 {
		h.Press("j")
	}
	if got := selected(t, h); got != "eight" {
		t.Fatalf("20× j selects %q, want the last task", got)
	}
	if got, want := visible(), "four five six seven eight"; got != want {
		t.Errorf("at the end the list shows %q, want %q", got, want)
	}

	h.Press("k")
	if got := selected(t, h); got != "seven" {
		t.Errorf("k after scrolling past the end selects %q, want seven", got)
	}
	if got, want := visible(), "four five six seven eight"; got != want {
		t.Errorf("k within the view scrolled it to %q, want %q", got, want)
	}
	h.Press("k", "k", "k", "k")
	if got, want := visible(), "three four five six seven"; got != want {
		t.Errorf("moving above the view shows %q, want %q", got, want)
	}

	h.Press("G")
	h.Resize(30, 6)
	if got := selected(t, h); got != "eight" {
		t.Errorf("after shrinking %q is selected, want eight", got)
	}
	if got := strings.Split(tile(h), "\n"); !strings.Contains(got[len(got)-2], "eight") {
		t.Errorf("shrinking hid the selection:\n%s", h.Screen())
	}
}

func TestWithoutATasksFileTheWidgetInvitesAnAddThatCreatesIt(t *testing.T) {
	root := t.TempDir()
	h := hosttest.New(t, hosttest.Options{Width: 50, Height: 7, Root: root, Config: `
[[widget]]
type = "tasks"
file = "lists/today.md"
`})
	if got, want := h.Line(1), "│ No tasks. Press a to add one.                  │"; got != want {
		t.Errorf("empty list %q, want %q", got, want)
	}
	if _, _, ok := h.Find("Tasks 0"); !ok {
		t.Errorf("empty list has no 0 count:\n%s", h.Screen())
	}

	h.Press("tab", "a")
	typeText(h, "Plan the week due tomorrow")
	h.Press("enter")
	b, err := os.ReadFile(filepath.Join(paths.Under(root).Data, "lists", "today.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), "- [ ] Plan the week due:2026-10-19\n"; got != want {
		t.Errorf("new file holds %q, want %q", got, want)
	}
	if !strings.Contains(h.Line(1), "Plan the week") || !strings.Contains(h.Line(1), "tomorrow") {
		t.Errorf("first row %q, want the new task due tomorrow", h.Line(1))
	}
}

func TestEditingTasksMdOutsideGhudUpdatesTheWidget(t *testing.T) {
	h := withTasks(t, "- [ ] Call the bank\n", hosttest.Options{Width: 40, Height: 7})

	writeTasks(t, h.Root, "- [ ] Call the bank\n- [ ] Water plants\n")
	if strings.Contains(tile(h), "Water plants") {
		t.Fatalf("Widget changed before the watcher reported the file:\n%s", tile(h))
	}
	h.Changed(tasksFile(h.Root))
	if !strings.Contains(tile(h), "○ Water plants") || !strings.Contains(h.Line(0), "Tasks 2") {
		t.Fatalf("Widget after the external edit:\n%s", tile(h))
	}
}

// typeText presses one key per character of s.
func typeText(h *hosttest.Host, s string) {
	for _, r := range s {
		k := string(r)
		if r == ' ' {
			k = "space"
		}
		h.Press(k)
	}
}

// selected is the title of the highlighted task, or "" when none is.
func selected(t *testing.T, h *hosttest.Host) string {
	t.Helper()
	got := ""
	for y, line := range strings.Split(h.Screen(), "\n") {
		if bg, ok := h.Bg(2, y); !ok || bg != theme.Sel {
			continue
		}
		if got != "" {
			t.Fatalf("rows %q and %q are both highlighted", got, line)
		}
		got = strings.TrimSpace(strings.Trim(line, "│ ○"))
		if i := strings.Index(got, "  "); i >= 0 {
			got = got[:i]
		}
	}
	return got
}
