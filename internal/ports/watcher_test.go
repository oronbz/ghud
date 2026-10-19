package ports

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type changed []string

// reports are what the watcher delivers, read by one goroutine calling Next in a loop.
type reports chan []string

func (r reports) next(t *testing.T) []string {
	t.Helper()
	select {
	case files := <-r:
		return files
	case <-time.After(5 * time.Second):
		t.Fatal("no change reported within 5s")
		return nil
	}
}

func (r reports) quiet(t *testing.T) {
	t.Helper()
	select {
	case files := <-r:
		t.Fatalf("unexpected change %v", files)
	case <-time.After(300 * time.Millisecond):
	}
}

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newTestWatcher(t *testing.T, files ...string) (*fsWatcher, reports) {
	t.Helper()
	w := newFSWatcher()
	t.Cleanup(func() { w.Close() })
	if err := w.Watch(files); err != nil {
		t.Fatal(err)
	}
	r := make(reports, 8)
	go func() {
		for {
			msg := w.Next(func(files []string) tea.Msg { return changed(files) })()
			if msg == nil {
				return
			}
			r <- msg.(changed)
		}
	}()
	return w, r
}

func TestWatcherReportsATempFileRenamedOverAWatchedFileOnce(t *testing.T) {
	dir := t.TempDir()
	tasks := filepath.Join(dir, "tasks.md")
	write(t, tasks, "- [ ] one\n")
	_, r := newTestWatcher(t, tasks)

	tmp := filepath.Join(dir, ".tasks.md-123")
	write(t, tmp, "- [ ] two\n")
	if err := os.Rename(tmp, tasks); err != nil {
		t.Fatal(err)
	}
	if got := r.next(t); !slices.Equal(got, []string{tasks}) {
		t.Fatalf("reported %v, want [%s]", got, tasks)
	}
	r.quiet(t)

	write(t, tasks, "- [ ] three\n")
	if got := r.next(t); !slices.Equal(got, []string{tasks}) {
		t.Fatalf("after a second write reported %v", got)
	}
}

func TestWatcherIgnoresOtherFilesInTheDirectory(t *testing.T) {
	dir := t.TempDir()
	_, r := newTestWatcher(t, filepath.Join(dir, "notes.md"))
	write(t, filepath.Join(dir, "other.md"), "x")
	r.quiet(t)
}

func TestWatcherReportsAFileCreatedInADirectoryThatDidNotExist(t *testing.T) {
	root := t.TempDir()
	notes := filepath.Join(root, "share", "ghud", "notes.md")
	_, r := newTestWatcher(t, notes)

	write(t, notes, "hello\n")
	if got := r.next(t); !slices.Equal(got, []string{notes}) {
		t.Fatalf("reported %v, want [%s]", got, notes)
	}
	write(t, notes, "hello again\n")
	if got := r.next(t); !slices.Equal(got, []string{notes}) {
		t.Fatalf("after a second write reported %v", got)
	}
}

func TestWatcherReportsFilesAsTheyWereSpelt(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	_, r := newTestWatcher(t, "config.toml")
	write(t, filepath.Join(dir, "config.toml"), "theme = \"ghud\"\n")
	if got := r.next(t); !slices.Equal(got, []string{"config.toml"}) {
		t.Fatalf("reported %v, want [config.toml]", got)
	}
}

func TestAWatchedDirectoryReportsEntriesComingAndGoing(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "plugins")
	staging := filepath.Join(plugins, ".staging")
	write(t, filepath.Join(staging, "new", "manifest"), "v1")
	write(t, filepath.Join(plugins, "old", "manifest"), "v0")
	_, r := newTestWatcher(t, plugins)

	if err := os.Rename(filepath.Join(staging, "new"), filepath.Join(plugins, "greet")); err != nil {
		t.Fatal(err)
	}
	if got := r.next(t); !slices.Equal(got, []string{plugins}) {
		t.Fatalf("an entry moved in: reported %v, want [%s]", got, plugins)
	}
	r.quiet(t)

	write(t, filepath.Join(plugins, "greet", "state.json"), "{}")
	write(t, filepath.Join(plugins, "greet", "manifest"), "v2")
	r.quiet(t)

	if err := os.Rename(filepath.Join(plugins, "old"), filepath.Join(staging, "old")); err != nil {
		t.Fatal(err)
	}
	if got := r.next(t); !slices.Equal(got, []string{plugins}) {
		t.Fatalf("an entry moved out: reported %v, want [%s]", got, plugins)
	}
}

func TestAWatchedDirectoryThatDidNotExistReportsItsFirstEntry(t *testing.T) {
	root := t.TempDir()
	plugins := filepath.Join(root, "share", "plugins")
	_, r := newTestWatcher(t, plugins)

	if err := os.MkdirAll(plugins, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := r.next(t); !slices.Equal(got, []string{plugins}) {
		t.Fatalf("the directory appearing: reported %v", got)
	}
	time.Sleep(50 * time.Millisecond)
	write(t, filepath.Join(plugins, "greet", "manifest"), "v1")
	if got := r.next(t); !slices.Equal(got, []string{plugins}) {
		t.Fatalf("its first entry: reported %v, want [%s]", got, plugins)
	}
}

func TestClosingTheWatcherReleasesAWaitingNext(t *testing.T) {
	w, _ := newTestWatcher(t, filepath.Join(t.TempDir(), "x"))
	got := make(chan tea.Msg, 1)
	go func() { got <- w.Next(func(files []string) tea.Msg { return changed(files) })() }()
	w.Close()
	select {
	case msg := <-got:
		if msg != nil {
			t.Fatalf("Next after Close delivered %v", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Next still waiting after Close")
	}
}
