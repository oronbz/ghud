package herdr_test

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/oronbz/visor/internal/closekey"
	"github.com/pelletier/go-toml/v2"
)

// herdr's default prefix, which "prefix+" in a snippet stands for.
const herdrPrefix = "ctrl+b"

func readme(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

type keyCommand struct {
	Key, Type, Command, Description, Width, Height string
}

// readmeBinds returns the [[keys.command]] entries of every ```toml block in
// the README, keyed by the block's type.
func readmeBinds(t *testing.T) map[string][]keyCommand {
	t.Helper()
	binds := map[string][]keyCommand{}
	for _, block := range regexp.MustCompile("(?s)```toml\n(.*?)```").FindAllStringSubmatch(readme(t), -1) {
		if !strings.Contains(block[1], "[[keys.command]]") {
			continue
		}
		var cfg struct {
			Keys struct{ Command []keyCommand }
		}
		if err := toml.Unmarshal([]byte(block[1]), &cfg); err != nil {
			t.Fatalf("snippet isn't valid TOML: %v\n%s", err, block[1])
		}
		for _, c := range cfg.Keys.Command {
			binds[c.Type] = append(binds[c.Type], c)
		}
	}
	return binds
}

// The README binds the toggle action this plugin's manifest declares.
func TestReadmeHerdrBindsRunThePluginsToggle(t *testing.T) {
	raw, err := os.ReadFile("herdr-plugin.toml")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		ID      string
		Actions []struct{ ID string }
	}
	if err := toml.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	want := m.ID + "." + m.Actions[0].ID
	var keys []string
	for _, c := range readmeBinds(t)["plugin_action"] {
		keys = append(keys, c.Key)
		if c.Command != want {
			t.Errorf("%s: runs %q, want %q", c.Key, c.Command, want)
		}
	}
	if want := []string{"cmd+shift+g", "prefix+d"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("plugin_action binds = %q, want %q", keys, want)
	}
}

// Each popup bind must be one of the --close-keys it passes, or the popup can
// be opened with a key that doesn't close it.
func TestReadmeHerdrPopupBindsCloseThePopupTheyOpen(t *testing.T) {
	popups := readmeBinds(t)["popup"]
	if len(popups) == 0 {
		t.Fatal("README has no popup bind")
	}
	for _, c := range popups {
		if c.Width != "100%" || c.Height != "100%" {
			t.Errorf("%s: %s×%s, want a 100%%×100%% popup", c.Key, c.Width, c.Height)
		}
		bind, err := closekey.Parse(strings.Replace(c.Key, "prefix+", herdrPrefix+" ", 1))
		if err != nil {
			t.Fatalf("%s: %v", c.Key, err)
		}
		if !containsSeq(closeKeysOf(t, c.Command), bind) {
			t.Errorf("%s: %q has no --close-key for it", c.Key, c.Command)
		}
	}
}

// closeKeysOf parses `visor --close-key a --close-key 'b c'` the way /bin/sh
// would for these simple words.
func closeKeysOf(t *testing.T, command string) []closekey.Seq {
	t.Helper()
	words := regexp.MustCompile(`'[^']*'|\S+`).FindAllString(command, -1)
	if len(words) == 0 || words[0] != "visor" {
		t.Fatalf("command %q doesn't run visor", command)
	}
	var seqs []closekey.Seq
	for i := 1; i < len(words); i += 2 {
		if words[i] != "--close-key" || i+1 == len(words) {
			t.Fatalf("command %q: want only --close-key <key> pairs", command)
		}
		seq, err := closekey.Parse(strings.Trim(words[i+1], "'"))
		if err != nil {
			t.Fatalf("command %q: %v", command, err)
		}
		seqs = append(seqs, seq)
	}
	return seqs
}

func containsSeq(seqs []closekey.Seq, want closekey.Seq) bool {
	for _, s := range seqs {
		if reflect.DeepEqual(s, want) {
			return true
		}
	}
	return false
}

func TestReadmeCoversInstallAndSkills(t *testing.T) {
	text := readme(t)
	for _, want := range []string{
		"A heads-up display for your terminal.",
		"brew install oronbz/tap/visor",
		"Intel",
		"herdr plugin install oronbz/visor/herdr",
		"visor skills install",
		"npx skills add oronbz/visor#vX.Y.Z -g",
		"visor plugin install",
		"[colors.dark]",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("README lacks %q", want)
		}
	}
	if strings.Contains(text, "—") {
		t.Error("README uses an em-dash")
	}
}
