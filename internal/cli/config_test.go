package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	toml "github.com/pelletier/go-toml/v2"

	"github.com/oronbz/ghud/internal/cli"
	"github.com/oronbz/ghud/internal/config"
	"github.com/oronbz/ghud/internal/widget"
	"github.com/oronbz/ghud/internal/widgets/calendar"
	"github.com/oronbz/ghud/internal/widgets/clock"
)

// sandbox is a temp home with env on top of HOME.
func sandbox(t *testing.T, env map[string]string, stdin string, args ...string) (cli.System, string, *bytes.Buffer, *bytes.Buffer) {
	home := t.TempDir()
	full := map[string]string{"HOME": home}
	for k, v := range env {
		full[k] = strings.ReplaceAll(v, "$HOME", home)
	}
	var stdout, stderr bytes.Buffer
	return cli.System{
		Args:   args,
		Getenv: func(k string) string { return full[k] },
		Stdin:  strings.NewReader(stdin),
		Stdout: &stdout,
		Stderr: &stderr,
		ProgramOptions: []tea.ProgramOption{
			tea.WithWindowSize(40, 9),
			tea.WithoutSignals(),
		},
	}, home, &stdout, &stderr
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func titledClock(title string) string {
	return "[[widget]]\ntype = \"clock\"\ntitle = \"" + title + "\"\n"
}

func TestConfigIsReadFromXDGConfigHome(t *testing.T) {
	s, home, stdout, stderr := sandbox(t, map[string]string{"XDG_CONFIG_HOME": "$HOME/xdg"}, "q")
	writeFile(t, filepath.Join(home, "xdg", "ghud", "config.toml"), titledClock("From XDG"))
	writeFile(t, filepath.Join(home, "Library", "Application Support", "ghud", "config.toml"), titledClock("From Library"))
	writeFile(t, filepath.Join(home, ".config", "ghud", "config.toml"), titledClock("From dot config"))

	if code := run(t, s); code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if out := ansi.Strip(stdout.String()); !strings.Contains(out, "From XDG") {
		t.Errorf("config not read from XDG_CONFIG_HOME:\n%s", out)
	}
}

func TestConfigFlagAndEnvOverrideThePath(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		args []string
		want string
	}{
		{"GHUD_CONFIG", map[string]string{"GHUD_CONFIG": "$HOME/env.toml"}, nil, "From env"},
		{"--config", nil, []string{"--config", "flag.toml"}, "From flag"},
		{"--config beats GHUD_CONFIG", map[string]string{"GHUD_CONFIG": "$HOME/env.toml"}, []string{"--config", "flag.toml"}, "From flag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, home, stdout, stderr := sandbox(t, tt.env, "q", tt.args...)
			for i, a := range s.Args {
				if strings.HasSuffix(a, ".toml") {
					s.Args[i] = filepath.Join(home, a)
				}
			}
			writeFile(t, filepath.Join(home, ".config", "ghud", "config.toml"), titledClock("From default"))
			writeFile(t, filepath.Join(home, "env.toml"), titledClock("From env"))
			writeFile(t, filepath.Join(home, "flag.toml"), titledClock("From flag"))

			if code := run(t, s); code != 0 {
				t.Fatalf("exit code %d, stderr: %s", code, stderr)
			}
			if out := ansi.Strip(stdout.String()); !strings.Contains(out, tt.want) {
				t.Errorf("want %q drawn:\n%s", tt.want, out)
			}
		})
	}
}

func TestConfigDefaultPrintsTheFullyCommentedReference(t *testing.T) {
	s, _, stdout, stderr := sandbox(t, map[string]string{"LANG": "en_US.UTF-8"}, "", "config", "default")

	if code := run(t, s); code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	ref := stdout.String()
	var doc map[string]any
	if err := toml.Unmarshal([]byte(ref), &doc); err != nil {
		t.Fatalf("not valid TOML: %v\n%s", err, ref)
	}
	if len(doc) != 0 {
		t.Errorf("not everything is commented out, got keys %v", doc)
	}
	for _, want := range []string{
		`#theme = "ghud"`,
		`#time_format = "12h"`,
		`#week_start = "sunday"`,
		`#mouse = true`,
		`#[[plugin]]`,
		`#type = "clock"`,
		`#type = "calendar"`,
		`#seconds = true`,
	} {
		if !strings.Contains(ref, want+"\n") {
			t.Errorf("reference lacks %q:\n%s", want, ref)
		}
	}
	for _, typ := range []widget.Type{clock.Type, calendar.Type} {
		for _, st := range typ.Settings {
			if !strings.Contains(ref, st.Why) {
				t.Errorf("reference lacks the why-line of %s.%s", typ.Name, st.Name)
			}
		}
	}
}

func TestConfigDefaultFollowsTheLocale(t *testing.T) {
	s, _, stdout, stderr := sandbox(t, map[string]string{"LANG": "de_DE.UTF-8"}, "", "config", "default")

	if code := run(t, s); code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	for _, want := range []string{`#time_format = "24h"`, `#week_start = "monday"`} {
		if !strings.Contains(stdout.String(), want+"\n") {
			t.Errorf("reference lacks %q:\n%s", want, stdout)
		}
	}
}

func TestTheReferenceUncommentedIsAValidConfig(t *testing.T) {
	s, home, stdout, stderr := sandbox(t, nil, "", "config", "default")
	if code := run(t, s); code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	var b strings.Builder
	for line := range strings.Lines(stdout.String()) {
		if strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "# ") && !strings.HasPrefix(line, "#\n") {
			line = line[1:]
		}
		b.WriteString(line)
	}
	path := filepath.Join(home, "uncommented.toml")
	writeFile(t, path, b.String())

	types := []widget.Type{clock.Type, calendar.Type}
	cfg, err := config.Load(path, types, config.Locale{})
	if err != nil {
		t.Fatalf("uncommented reference doesn't parse: %v\n%s", err, b.String())
	}
	if len(cfg.Problems) > 0 {
		t.Errorf("uncommented reference has problems %v\n%s", cfg.Problems, b.String())
	}
	if len(cfg.Widgets) != len(types) {
		t.Errorf("uncommented reference places %d Widgets, want one per type", len(cfg.Widgets))
	}
}

func TestConfigInitWritesTheReferenceOnce(t *testing.T) {
	s, home, stdout, stderr := sandbox(t, nil, "", "config", "init")
	if code := run(t, s); code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	path := filepath.Join(home, ".config", "ghud", "config.toml")
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), `#type = "clock"`) {
		t.Errorf("init didn't write the reference:\n%s", written)
	}
	if !strings.Contains(stdout.String(), path) {
		t.Errorf("init doesn't say where it wrote: %q", stdout)
	}

	writeFile(t, path, "# mine\n")
	stdout.Reset()
	stderr.Reset()
	if code := run(t, s); code != 1 {
		t.Errorf("second init exit code %d, want 1", code)
	}
	if got, _ := os.ReadFile(path); string(got) != "# mine\n" {
		t.Errorf("second init overwrote the config:\n%s", got)
	}
	if !strings.Contains(stderr.String(), "already exists") {
		t.Errorf("second init doesn't explain: %q", stderr)
	}
}

func TestLaunchingWithNoConfigWritesNothing(t *testing.T) {
	s, home, _, stderr := sandbox(t, nil, "q")
	if code := run(t, s); code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("launch wrote %v into the home", entries)
	}
}
