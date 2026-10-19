package ports

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"
)

// settle is how long a file must stay quiet before its change is reported,
// so that one save (often several events) is reported once.
const settle = 75 * time.Millisecond

// fsWatcher watches the directories holding the files, not the files:
// temp-file-plus-rename writes replace a file's inode, which ends a watch on it.
type fsWatcher struct {
	start sync.Once
	w     *fsnotify.Watcher
	err   error
	done  chan struct{}
	out   chan []string

	mu sync.Mutex
	// files maps each watched file's clean absolute path to its spelling in Watch.
	files map[string]string
	dirs  map[string]bool
}

func newFSWatcher() *fsWatcher {
	return &fsWatcher{done: make(chan struct{}), out: make(chan []string), files: map[string]string{}, dirs: map[string]bool{}}
}

func (f *fsWatcher) Watch(files []string) error {
	f.start.Do(func() {
		if f.w, f.err = fsnotify.NewWatcher(); f.err == nil {
			go f.loop()
		}
	})
	if f.err != nil {
		return f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	clear(f.files)
	var errs []error
	for _, file := range files {
		abs, err := filepath.Abs(file)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		f.files[abs] = file
	}
	_, err := f.addDirs()
	return errors.Join(append(errs, err)...)
}

// addDirs watches each file's directory or, until it exists, its nearest
// existing ancestor, and stops watching directories no file needs. It
// returns the directories it started watching.
func (f *fsWatcher) addDirs() ([]string, error) {
	var want map[string]bool
	var added []string
	var errs []error
	// A directory created inside an ancestor before its watch was in place
	// sends no event, so look again after every watch added until none is.
	for grew := true; grew; {
		grew = false
		want = f.wantDirs()
		for d := range want {
			if f.dirs[d] {
				continue
			}
			if err := f.w.Add(d); err != nil {
				errs = append(errs, err)
				continue
			}
			f.dirs[d] = true
			added = append(added, d)
			grew = true
		}
	}
	for d := range f.dirs {
		if !want[d] {
			_ = f.w.Remove(d)
			delete(f.dirs, d)
		}
	}
	return added, errors.Join(errs...)
}

// wantDirs are the directories addDirs should be watching right now.
func (f *fsWatcher) wantDirs() map[string]bool {
	want := map[string]bool{}
	for abs := range f.files {
		if fi, err := os.Stat(abs); err == nil && fi.IsDir() {
			want[abs] = true
		}
		d := filepath.Dir(abs)
		for {
			if fi, err := os.Stat(d); err == nil && fi.IsDir() {
				break
			}
			up := filepath.Dir(d)
			if up == d {
				break
			}
			d = up
		}
		want[d] = true
	}
	return want
}

func (f *fsWatcher) loop() {
	pending := map[string]bool{}
	var quiet <-chan time.Time
	var out chan []string
	for {
		var ready []string
		if out != nil {
			for p := range pending {
				ready = append(ready, p)
			}
			slices.Sort(ready)
		}
		select {
		case ev, ok := <-f.w.Events:
			if !ok {
				return
			}
			if hits := f.hits(ev); len(hits) > 0 {
				for _, h := range hits {
					pending[h] = true
				}
				quiet = time.After(settle)
			}
		case _, ok := <-f.w.Errors:
			if !ok {
				return
			}
		case <-quiet:
			quiet, out = nil, f.out
		case out <- ready:
			clear(pending)
			out = nil
		case <-f.done:
			return
		}
	}
}

// hits are the watched files an event means changed, by their Watch spelling.
func (f *fsWatcher) hits(ev fsnotify.Event) []string {
	if ev.Op == fsnotify.Chmod {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	name := filepath.Clean(ev.Name)
	var added []string
	if ev.Has(fsnotify.Create) {
		added, _ = f.addDirs()
	}
	if file, ok := f.files[name]; ok {
		return []string{file}
	}
	// A watched directory changes when an entry comes or goes, not when one is written to.
	if dir, ok := f.files[filepath.Dir(name)]; ok && f.dirs[filepath.Dir(name)] && ev.Has(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) {
		return []string{dir}
	}
	// A directory on the way to a watched file appeared; the file may have
	// been written into it before the new watch was in place.
	var hits []string
	for abs, file := range f.files {
		for _, d := range added {
			if filepath.Dir(abs) == d || abs == d {
				if _, err := os.Stat(abs); err == nil {
					hits = append(hits, file)
				}
			}
		}
	}
	return hits
}

func (f *fsWatcher) Next(msg func([]string) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		select {
		case files := <-f.out:
			return msg(files)
		case <-f.done:
			return nil
		}
	}
}

// Close stops watching and releases every waiting Next.
func (f *fsWatcher) Close() error {
	select {
	case <-f.done:
		return nil
	default:
	}
	close(f.done)
	if f.w != nil {
		return f.w.Close()
	}
	return nil
}
