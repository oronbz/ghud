package jira_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/hosttest"
)

const ghudAccount = "jira:acme.atlassian.net:oron@acme.io"

// keyring is a keyring holding secrets, keyed "service account".
func keyring(t *testing.T, secrets map[string]string) *hosttest.Keyring {
	t.Helper()
	k := &hosttest.Keyring{}
	for sa, secret := range secrets {
		service, account, _ := strings.Cut(sa, " ")
		if err := k.Set(service, account, secret); err != nil {
			t.Fatal(err)
		}
	}
	return k
}

// netrc writes a .netrc into a fresh home dir and returns the dir.
func netrc(t *testing.T, text string) string {
	t.Helper()
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".netrc"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestTheTokenComesFromTheEnvThenGhudsKeyringThenJiraCLIThenNetrc(t *testing.T) {
	both := keyring(t, map[string]string{"ghud " + ghudAccount: "ghud-token", "jira-cli oron@acme.io": "cli-token"})
	cliOnly := keyring(t, map[string]string{"jira-cli oron@acme.io": "cli-token"})
	home := netrc(t, "machine github.com login me password nope\n"+
		"machine acme.atlassian.net\n  login oron@acme.io\n  password netrc-token\n")
	for _, tc := range []struct {
		name    string
		env     map[string]string
		keyring *hosttest.Keyring
		want    string
	}{
		{"JIRA_API_TOKEN wins", map[string]string{"JIRA_API_TOKEN": "env-token", "HOME": home}, both, "env-token"},
		{"then ghud's keyring item", map[string]string{"HOME": home}, both, "ghud-token"},
		{"then jira-cli's keychain item", map[string]string{"HOME": home}, cliOnly, "cli-token"},
		{"then .netrc", map[string]string{"HOME": home}, &hosttest.Keyring{}, "netrc-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			j := queue(t)
			withJira(t, j, hosttest.Options{Env: tc.env, Keyring: tc.keyring})
			s := j.searches()
			if len(s) == 0 {
				t.Fatal("Jira wasn't searched")
			}
			if got, want := s[0].Auth, basic("oron@acme.io", tc.want); got != want {
				t.Errorf("Authorization %q, want %q (%s)", got, want, tc.want)
			}
		})
	}
}

func TestANetrcEntryForAnotherSiteOrLoginIsIgnored(t *testing.T) {
	home := netrc(t, "machine other.atlassian.net login oron@acme.io password other\n"+
		"machine acme.atlassian.net login someone@acme.io password theirs\n")
	j := queue(t)
	h := withJira(t, j, hosttest.Options{Env: map[string]string{"HOME": home}})
	if s := j.searches(); len(s) != 0 {
		t.Errorf("searched with %v, want no token found", s[0].Auth)
	}
	if !strings.Contains(h.Screen(), "needs setup") {
		t.Errorf("want needs setup:\n%s", h.Screen())
	}
}

func TestWithNoTokenJiraNeedsSetupAndNeverCallsJira(t *testing.T) {
	j := queue(t)
	h := withJira(t, j, hosttest.Options{Env: map[string]string{}})
	screen := h.Screen()
	for _, want := range []string{"needs setup", "JIRA_API_TOKEN"} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen lacks %q:\n%s", want, screen)
		}
	}
	if r := j.Requests(); len(r) != 0 {
		t.Errorf("Jira got %v with no token", r)
	}
}

func TestWithNoSiteJiraNeedsSetup(t *testing.T) {
	j := queue(t)
	h := withJira(t, j, hosttest.Options{Config: "[[widget]]\ntype = \"jira\"\n"})
	screen := h.Screen()
	for _, want := range []string{"needs setup", "site"} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen lacks %q:\n%s", want, screen)
		}
	}
	if r := j.Requests(); len(r) != 0 {
		t.Errorf("Jira got %v with no site", r)
	}
}
