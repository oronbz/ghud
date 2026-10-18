// Package plugintest builds the fixture Plugin, a tiny Go program in
// testdata that plays scripted protocol behaviours, and lays out Plugin
// directories for tests.
package plugintest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

var (
	build    sync.Once
	binary   string
	buildErr error
)

// Binary is the fixture Plugin's executable, built once per test process.
func Binary(t testing.TB) string {
	t.Helper()
	build.Do(func() {
		_, here, _, _ := runtime.Caller(0)
		dir, err := os.MkdirTemp("", "ghud-fixture-")
		if err != nil {
			buildErr = err
			return
		}
		binary = filepath.Join(dir, "fixture")
		if runtime.GOOS == "windows" {
			binary += ".exe"
		}
		cmd := exec.Command("go", "build", "-o", binary, ".")
		cmd.Dir = filepath.Join(filepath.Dir(here), "testdata", "fixture")
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("building the fixture Plugin: %v\n%s", err, out)
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return binary
}

// Options shape a fixture Plugin's manifest.
type Options struct {
	// Name defaults to "fixture", which is also its Widget type's name.
	Name string
	// Protocol is the version the manifest declares; zero is 1.
	Protocol int
	// Args are passed to the fixture, such as "-protocol", "2".
	Args []string
	// Refresh is the Widget type's default interval; "" is "1m".
	Refresh string
	// Default places a Widget of the type when config.toml places none.
	Default bool
	// Mode is the `mode` setting's default; "" is "items".
	Mode string
	// Command replaces the built fixture (and Args) as the command.
	Command []string
	// Build are the manifest's [[build]] steps.
	Build [][]string
}

// Write lays out a fixture Plugin in dir: its ghud-plugin.toml, whose
// command runs the built fixture. It returns dir.
func Write(t testing.TB, dir string, o Options) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ghud-plugin.toml"), []byte(Manifest(t, o)), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// Manifest is the ghud-plugin.toml Write lays out.
func Manifest(t testing.TB, o Options) string {
	t.Helper()
	if o.Name == "" {
		o.Name = "fixture"
	}
	if o.Protocol == 0 {
		o.Protocol = 1
	}
	if o.Refresh == "" {
		o.Refresh = "1m"
	}
	if o.Mode == "" {
		o.Mode = "items"
	}
	if o.Command == nil {
		o.Command = append([]string{Binary(t)}, o.Args...)
	}
	var build strings.Builder
	for _, step := range o.Build {
		fmt.Fprintf(&build, "\n[[build]]\nrun = [%s]\n", quoted(step))
	}
	return fmt.Sprintf(`name = %q
description = "Scripted protocol behaviours for tests"
protocol = %d
command = [%s]

[[widget]]
type = %q
title = "Fixture"
description = "Shows what the fixture Plugin is told to"
refresh = %q
floor = "10s"
default = %t

[[widget.setting]]
name = "greeting"
kind = "string"
default = "hello"
description = "Words in front of every Item"

[[widget.setting]]
name = "mode"
kind = "string"
default = %q
description = "What refreshes return"
choices = ["items", "lines", "raw-colour", "malformed", "unknown-field", "setup", "error"]
%s`, o.Name, o.Protocol, quoted(o.Command), o.Name, o.Refresh, o.Default, o.Mode, build.String())
}

func quoted(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		q[i] = fmt.Sprintf("%q", a)
	}
	return strings.Join(q, ", ")
}
