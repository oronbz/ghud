package plugin_test

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/oronbz/ghud/internal/plugin"
	"github.com/oronbz/ghud/internal/plugin/plugintest"
	"github.com/oronbz/ghud/internal/widget"
)

const goodManifest = `name = "demo"
protocol = 1
command = "./demo"

[[widget]]
type = "demo"
title = "Demo"
description = "A demo"
`

func writeManifest(t *testing.T, dir, src string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, plugin.ManifestFile), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestBadManifestsAreProblemsNamingTheFile(t *testing.T) {
	for name, c := range map[string]struct{ src, want string }{
		"no command":       {strings.Replace(goodManifest, `command = "./demo"`, "", 1), "missing property 'command'"},
		"unknown key":      {goodManifest + "colour = \"red\"\n", "additional properties 'colour' not allowed"},
		"bad refresh":      {goodManifest + "refresh = \"soon\"\n", "widget.0.refresh"},
		"default mismatch": {goodManifest + "[[widget.setting]]\nname = \"n\"\nkind = \"int\"\ndefault = \"three\"\ndescription = \"a number\"\n", "widget.0.setting.0.default"},
		"reserved setting": {goodManifest + "[[widget.setting]]\nname = \"title\"\nkind = \"string\"\ndefault = \"\"\ndescription = \"a title\"\n", "`title` is a key every Widget takes"},
		"bad choice":       {goodManifest + "[[widget.setting]]\nname = \"u\"\nkind = \"string\"\ndefault = \"kelvin\"\nchoices = [\"c\", \"f\"]\ndescription = \"units\"\n", "default must be one of"},
		"not toml":         {"name = ", "line 1"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := writeManifest(t, t.TempDir(), c.src)
			types, problems := plugin.NewRegistry().Discover([]string{dir}, nil)
			if len(types) != 0 || len(problems) != 1 {
				t.Fatalf("types %v, problems %q", types, problems)
			}
			if !strings.HasPrefix(problems[0], filepath.Join(dir, plugin.ManifestFile)+": ") || !strings.Contains(problems[0], c.want) {
				t.Errorf("problem = %q, want the file and %q", problems[0], c.want)
			}
		})
	}
}

func TestTakenWidgetTypesAreLeftOut(t *testing.T) {
	root := t.TempDir()
	a := writeManifest(t, filepath.Join(root, "a"), goodManifest)
	b := writeManifest(t, filepath.Join(root, "b"), strings.Replace(goodManifest, `name = "demo"`, `name = "other"`, 1))
	clock := writeManifest(t, filepath.Join(root, "c"), strings.NewReplacer(`name = "demo"`, `name = "c"`, `type = "demo"`, `type = "clock"`).Replace(goodManifest))
	types, problems := plugin.NewRegistry().Discover([]string{a, b, clock}, []string{"clock"})
	if len(types) != 1 || types[0].Name != "demo" {
		t.Errorf("types = %v, want only the first demo", types)
	}
	if len(problems) != 2 || !strings.Contains(problems[0], "Widget type `demo` is taken") || !strings.Contains(problems[1], "Widget type `clock` is taken") {
		t.Errorf("problems = %q", problems)
	}
}

func TestDirsListsInstalledPluginsThenListedPaths(t *testing.T) {
	plugins := t.TempDir()
	writeManifest(t, filepath.Join(plugins, "b"), goodManifest)
	writeManifest(t, filepath.Join(plugins, "a"), goodManifest)
	if err := os.MkdirAll(filepath.Join(plugins, "not-a-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := plugin.Dirs(plugins, []string{"/dev/plugin"})
	want := []string{filepath.Join(plugins, "a"), filepath.Join(plugins, "b"), "/dev/plugin"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("dirs = %v, want %v", got, want)
	}
}

// The fixture logs every message it reads and writes; each must match the
// shipped protocol schema, as must the fixture's own manifest.
func TestTheProtocolSchemaDescribesARealSession(t *testing.T) {
	log := filepath.Join(t.TempDir(), "session.jsonl")
	r, typ := fixture(t, plugintest.Options{Args: []string{"-log", log}})
	c := &clock{now: t0}
	w := open(t, typ, "fx", c, nil)
	got := listen(w)
	refresh(t, w)
	w.(widget.Observer).Observe(widget.Shown{Size: widget.Size{W: 30, H: 5}, Focused: true, Visible: true})
	got.next(t)
	for _, inv := range []widget.Invocation{{Action: "done", Item: "alpha"}, {Action: "note", Text: "x", Base: "y"}, {Action: "nope"}, {Action: "move", Item: "alpha", Choice: "done"}, {Action: "greet", Text: "yo"}, {Action: "chain"}} {
		invoke(t, w, inv)
	}
	w.(widget.Chooser).Choices(context.Background(), widget.Invocation{Action: "move", Item: "alpha"})
	lines := open(t, typ, "lines", c, widget.Settings{"mode": "lines"})
	refresh(t, lines)
	refresh(t, open(t, typ, "setup", c, widget.Settings{"mode": "setup"}))
	refresh(t, open(t, typ, "error", c, widget.Settings{"mode": "error"}))
	lines.(widget.Closer).Close()
	var b []byte
	for deadline := time.Now().Add(10 * time.Second); !bytes.Contains(b, []byte("widget/close")) && time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		b, _ = os.ReadFile(log)
	}
	r.Close()
	methods := map[string]bool{}
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(nil, 1<<24)
	for sc.Scan() {
		msg, err := jsonschema.UnmarshalJSON(bytes.NewReader(sc.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		if err := plugin.Validate("protocol.json", msg); err != nil {
			t.Errorf("%s: %v", sc.Bytes(), err)
		}
		if m, ok := msg.(map[string]any)["method"].(string); ok {
			methods[m] = true
		}
		if res, ok := msg.(map[string]any)["result"].(map[string]any); ok {
			def := "refreshResult"
			switch {
			case res["protocol"] != nil:
				def = "initializeResult"
			case res["choices"] != nil:
				def = "choicesResult"
			case res["toast"] != nil, res["then"] != nil:
				def = "invokeResult"
			}
			if err := plugin.Validate("protocol.json#/$defs/"+def, res); err != nil {
				t.Errorf("%s against %s: %v", sc.Bytes(), def, err)
			}
		}
	}
	for _, m := range []string{"initialize", "widget/open", "widget/frame", "widget/refresh", "widget/update", "action/invoke", "action/choices", "widget/close"} {
		if !methods[m] {
			t.Errorf("the session never sent %s", m)
		}
	}
}

func TestTheFixtureManifestMatchesTheSchema(t *testing.T) {
	dir := plugintest.Write(t, t.TempDir(), plugintest.Options{})
	if _, err := plugin.ReadManifest(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range plugin.SchemaNames {
		if len(plugin.Schema(name)) == 0 {
			t.Errorf("schema %s isn't embedded", name)
		}
	}
}
