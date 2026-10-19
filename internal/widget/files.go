package widget

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"

	"github.com/oronbz/visor/internal/paths"
)

// DataFile resolves a Widget's `file` setting: empty means def in the data
// dir, a relative path is under the data dir, and `~/` is the home dir
// (the data dir when there's no home).
func (c Config) DataFile(file, def string) string {
	if file == "" {
		return filepath.Join(c.DataDir, def)
	}
	home := paths.Home(c.Ports.Getenv)
	if home == "" {
		home = c.DataDir
	}
	if file = paths.Expand(file, home); !filepath.IsAbs(file) {
		return filepath.Join(c.DataDir, file)
	}
	return file
}

// WriteFile replaces path atomically (temp file plus rename), so a reader
// never sees half of it. An existing file keeps its permissions.
func WriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	mode := fs.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// EditLock is the lock file serialising edits of file across visor
// instances, named for kind. It lives in the data dir so synced folders
// don't collect lock files.
func (c Config) EditLock(kind, file string) string {
	return LockFile(filepath.Join(c.DataDir, "locks"), kind, file)
}

func LockFile(dir, kind, file string) string {
	sum := sha256.Sum256([]byte(file))
	return filepath.Join(dir, kind+"-"+hex.EncodeToString(sum[:8])+".lock")
}

func Lock(path string) (unlock func(), err error) {
	l, err := lockAt(path)
	if err != nil {
		return nil, err
	}
	if err := l.Lock(); err != nil {
		return nil, err
	}
	return func() { _ = l.Unlock() }, nil
}

// TryLock never waits: ok is false while someone else holds the lock.
func TryLock(path string) (unlock func(), ok bool, err error) {
	l, err := lockAt(path)
	if err != nil {
		return nil, false, err
	}
	if ok, err := l.TryLock(); !ok || err != nil {
		return nil, false, err
	}
	return func() { _ = l.Unlock() }, true, nil
}

func lockAt(path string) (*flock.Flock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return flock.New(path), nil
}

// ReadFile reads a missing file as empty.
func ReadFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return string(b), err
}
