// Package paths resolves ghud's config, data and cache directories. ghud does
// XDG itself on every OS because os.UserConfigDir ignores XDG_CONFIG_HOME on macOS.
package paths

import (
	"errors"
	"path/filepath"
)

type Paths struct {
	Config string
	Data   string
	Cache  string
}

// Under puts all three directories inside root, for tests and sandboxes.
func Under(root string) Paths {
	return Paths{
		Config: filepath.Join(root, "config", "ghud"),
		Data:   filepath.Join(root, "data", "ghud"),
		Cache:  filepath.Join(root, "cache", "ghud"),
	}
}

// Resolve applies XDG base directories, then the OS default: ~/.config,
// ~/.local/share and ~/.cache, or %AppData% and %LocalAppData% on Windows.
func Resolve(getenv func(string) string, goos string) (Paths, error) {
	dir := func(xdg, unixDefault, winVar, winSub string) (string, error) {
		if v := getenv(xdg); filepath.IsAbs(v) {
			return filepath.Join(v, "ghud"), nil
		}
		if goos == "windows" {
			base := getenv(winVar)
			if base == "" {
				base = getenv("AppData")
			}
			if base == "" {
				return "", errors.New("paths: %AppData% is not set")
			}
			return filepath.Join(base, "ghud", winSub), nil
		}
		home := getenv("HOME")
		if home == "" {
			return "", errors.New("paths: $HOME is not set")
		}
		return filepath.Join(home, unixDefault, "ghud"), nil
	}
	var p Paths
	var err error
	if p.Config, err = dir("XDG_CONFIG_HOME", ".config", "AppData", ""); err != nil {
		return Paths{}, err
	}
	if p.Data, err = dir("XDG_DATA_HOME", filepath.Join(".local", "share"), "LocalAppData", "data"); err != nil {
		return Paths{}, err
	}
	if p.Cache, err = dir("XDG_CACHE_HOME", ".cache", "LocalAppData", "cache"); err != nil {
		return Paths{}, err
	}
	return p, nil
}
