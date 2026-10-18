package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// RecordFile, in an installed Plugin's directory, says where it came from.
const RecordFile = ".ghud-source.json"

// Record is where `ghud plugin install` got a Plugin.
type Record struct {
	// Source is owner/repo or owner/repo/subdir on GitHub.
	Source string `json:"source"`
	// Ref is the branch, tag or commit asked for; empty is the default branch.
	Ref    string `json:"ref,omitempty"`
	Commit string `json:"commit"`
}

// ReadRecord reads the Record of the Plugin installed in dir.
func ReadRecord(dir string) (Record, error) {
	var r Record
	b, err := os.ReadFile(filepath.Join(dir, RecordFile))
	if err != nil {
		return r, err
	}
	return r, json.Unmarshal(b, &r)
}

// Write saves r in dir.
func (r Record) Write(dir string) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, RecordFile), append(b, '\n'), 0o644)
}

// InstallCommand is the command that installs the Plugin again.
func (r Record) InstallCommand() string {
	cmd := "ghud plugin install " + r.Source
	if r.Ref != "" {
		cmd += " --ref " + r.Ref
	}
	return cmd
}
