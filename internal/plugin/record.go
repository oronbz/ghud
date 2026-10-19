package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/oronbz/visor/internal/widget"
)

// RecordFile, in an installed Plugin's directory, says where it came from.
const RecordFile = ".visor-source.json"

// Record is where `visor plugin install` got a Plugin.
type Record struct {
	// Source is owner/repo or owner/repo/subdir on GitHub.
	Source string `json:"source"`
	// Ref is the branch, tag or commit asked for; empty is the default branch.
	Ref    string `json:"ref,omitempty"`
	Commit string `json:"commit"`
}

func ReadRecord(dir string) (Record, error) {
	var r Record
	b, err := os.ReadFile(filepath.Join(dir, RecordFile))
	if err != nil {
		return r, err
	}
	return r, json.Unmarshal(b, &r)
}

func (r Record) Write(dir string) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, RecordFile), append(b, '\n'), 0o644)
}

// buried is hidden so visor doesn't read it as a Plugin.
const buried = ".uninstalled"

// Tombstone is what visor keeps of an uninstalled Plugin, so its Widgets can
// name the install command in instances started after the uninstall.
type Tombstone struct {
	Name    string         `json:"name"`
	Record  Record         `json:"record"`
	Widgets []BuriedWidget `json:"widgets"`
}

// BuriedWidget is a Widget type of an uninstalled Plugin.
type BuriedWidget struct {
	Type  string `json:"type"`
	Title string `json:"title"`
}

func Bury(pluginsDir string, m Manifest, rec Record) error {
	t := Tombstone{Name: m.Name, Record: rec}
	for _, wt := range m.Widgets {
		t.Widgets = append(t.Widgets, BuriedWidget{Type: wt.Type, Title: wt.Title})
	}
	b, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Join(pluginsDir, buried)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, m.Name+".json"), append(b, '\n'), 0o644)
}

func Unbury(pluginsDir, name string) error {
	err := os.Remove(filepath.Join(pluginsDir, buried, name+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Buried are stand-ins for the Widget types of the Plugins uninstalled from
// pluginsDir, leaving out type names in taken.
func Buried(pluginsDir string, taken []string) []widget.Type {
	files, _ := filepath.Glob(filepath.Join(pluginsDir, buried, "*.json"))
	var types []widget.Type
	for _, f := range files {
		b, err := os.ReadFile(f)
		var t Tombstone
		if err != nil || json.Unmarshal(b, &t) != nil {
			continue
		}
		for _, wt := range t.Widgets {
			if !slices.Contains(taken, wt.Type) {
				types = append(types, widget.Type{Name: wt.Type, Title: wt.Title, OptIn: true, Missing: missing(t.Name, t.Record)})
			}
		}
	}
	return types
}

func missing(name string, rec Record) string {
	hint := fmt.Sprintf("Plugin `%s` isn't installed", name)
	if rec.Source != "" {
		hint += "; run `" + rec.InstallCommand() + "`"
	}
	return hint
}

func (r Record) InstallCommand() string {
	cmd := "visor plugin install " + r.Source
	if r.Ref != "" {
		cmd += " --ref " + r.Ref
	}
	return cmd
}
