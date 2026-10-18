package widget

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DataFile resolves a Widget's `file` setting: empty means def in the data
// dir, a relative path is under the data dir, and `~/` is the home dir.
func (c Config) DataFile(file, def string) string {
	switch {
	case file == "":
		return filepath.Join(c.DataDir, def)
	case strings.HasPrefix(file, "~/"):
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, file[2:])
		}
	case !filepath.IsAbs(file):
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
