package herdr_test

import (
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/closekey"
	"github.com/pelletier/go-toml/v2"
)

// herdr's default prefix, which "prefix+d" in the snippet stands for.
const herdrPrefix = "ctrl+b"

func readme(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../README.md")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// Each bind in the README snippet must be one of the --close-keys it passes,
// or the popup can be opened with a key that doesn't close it.
func TestReadmeHerdrBindsCloseThePopupTheyOpen(t *testing.T) {
	var snippet string
	for _, block := range regexp.MustCompile("(?s)```toml\n(.*?)```").FindAllStringSubmatch(readme(t), -1) {
		if strings.Contains(block[1], "[[keys.command]]") {
			snippet = block[1]
		}
	}
	if snippet == "" {
		t.Fatal("README has no ```toml block with [[keys.command]]")
	}
	var cfg struct {
		Keys struct {
			Command []struct {
				Key, Type, Command, Description, Width, Height string
			}
		}
	}
	if err := toml.Unmarshal([]byte(snippet), &cfg); err != nil {
		t.Fatalf("snippet isn't valid TOML: %v", err)
	}
	var keys []string
	for _, c := range cfg.Keys.Command {
		keys = append(keys, c.Key)
		if c.Type != "popup" || c.Width != "100%" || c.Height != "100%" {
			t.Errorf("%s: type %q %s×%s, want a 100%%×100%% popup", c.Key, c.Type, c.Width, c.Height)
		}
		closeKeys := closeKeysOf(t, c.Command)
		bind, err := closekey.Parse(strings.Replace(c.Key, "prefix+", herdrPrefix+" ", 1))
		if err != nil {
			t.Fatalf("%s: %v", c.Key, err)
		}
		if !containsSeq(closeKeys, bind) {
			t.Errorf("%s: %q has no --close-key for it", c.Key, c.Command)
		}
	}
	if want := []string{"cmd+shift+g", "prefix+d"}; !reflect.DeepEqual(keys, want) {
		t.Errorf("binds = %q, want %q", keys, want)
	}
}

// closeKeysOf parses `ghud --close-key a --close-key 'b c'` the way /bin/sh
// would for these simple words.
func closeKeysOf(t *testing.T, command string) []closekey.Seq {
	t.Helper()
	words := regexp.MustCompile(`'[^']*'|\S+`).FindAllString(command, -1)
	if len(words) == 0 || words[0] != "ghud" {
		t.Fatalf("command %q doesn't run ghud", command)
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
		"god's-eye HUD for your day",
		"brew install oronbz/tap/ghud",
		"Intel",
		"herdr plugin install oronbz/ghud/herdr",
		"ghud skills install",
		"npx skills add oronbz/ghud#vX.Y.Z -g",
		"ghud plugin install",
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
