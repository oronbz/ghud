package layout

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// FileName is the Layout file in the config dir. ghud owns it (ADR 0003).
const FileName = "layout.json"

// file keeps the Layout under "default" so named Layouts can come later.
type file struct {
	Default *Layout `json:"default"`
}

// Load reads the saved Layout from configDir. It returns an error wrapping
// os.ErrNotExist when nothing has been saved yet.
func Load(configDir string) (Layout, error) {
	b, err := os.ReadFile(filepath.Join(configDir, FileName))
	if err != nil {
		return Layout{}, err
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return Layout{}, err
	}
	if f.Default == nil {
		return Layout{}, errors.New(`layout.json: no "default" Layout`)
	}
	return *f.Default, nil
}

// Save writes l as the saved Layout in configDir, atomically: a reader sees
// the old file or the new one, never a partial write.
func Save(configDir string, l Layout) error {
	b, err := json.MarshalIndent(file{Default: &l}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(configDir, ".layout-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(configDir, FileName))
}
