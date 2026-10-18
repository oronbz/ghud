package widget

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/gofrs/flock"
)

// DataFile resolves a Widget's `file` setting: empty means def in the data
// dir, a relative path is under the data dir, and `~/` is the home dir
// (the data dir when there's no HOME).
func (c Config) DataFile(file, def string) string {
	switch {
	case file == "":
		return filepath.Join(c.DataDir, def)
	case strings.HasPrefix(file, "~/"):
		return filepath.Join(c.home(), file[2:])
	case !filepath.IsAbs(file):
		return filepath.Join(c.DataDir, file)
	}
	return file
}

func (c Config) home() string {
	if c.Ports.Getenv != nil {
		for _, k := range []string{"HOME", "USERPROFILE"} {
			if h := c.Ports.Getenv(k); h != "" {
				return h
			}
		}
	}
	return c.DataDir
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

// EditLock is the lock file serialising edits of file across ghud
// instances, named for kind. It lives in the data dir so synced folders
// don't collect lock files.
func (c Config) EditLock(kind, file string) string {
	sum := sha256.Sum256([]byte(file))
	return filepath.Join(c.DataDir, "locks", kind+"-"+hex.EncodeToString(sum[:8])+".lock")
}

// Lock blocks until it holds the lock file at path.
func Lock(path string) (unlock func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	l := flock.New(path)
	if err := l.Lock(); err != nil {
		return nil, err
	}
	return func() { _ = l.Unlock() }, nil
}

// ReadFile reads path as text; a missing file is empty.
func ReadFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return string(b), err
}
