package tasks_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/paths"
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
