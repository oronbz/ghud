package jira_test

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/hosttest"
)

const (
	cloudTokenPage = "https://id.atlassian.com/manage-profile/security/api-tokens"
	setupHint      = "Set up Jira"
	apiToken       = "test-jira-api-token"
	handWritten    = `# my dashboard
theme = "ghud"   # keep it

[[widget]]
type = "clock"

# work queue
[[widget]]
type = "jira"
title = "Work"
jql = "project = OPS"
`
)

// configText is config.toml as it is on disk.
func configText(t *testing.T, h *hosttest.Host) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(h.Paths.Config, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// setUp answers the Set up Jira prompts in the focused Widget: site, email
// (each replacing the prefilled text) and the token.
func setUp(h *hosttest.Host, site, email, token string) {
	h.Press("enter", "ctrl+u")
	h.Type(site)
	h.Press("enter", "ctrl+u")
	h.Type(email)
	h.Press("enter")
	h.Type(token)
	h.Press("enter")
}

func TestSetUpJiraPromptsForSiteEmailAndTokenThenLoads(t *testing.T) {
	j := queue(t)
	j.queries["project = OPS"] = j.issues
	h := withJira(t, j, hosttest.Options{Width: 100, Height: 12, Env: map[string]string{}, Config: handWritten})
	if s := h.Screen(); !strings.Contains(s, "needs setup") || !strings.Contains(s, setupHint) {
		t.Fatalf("want needs setup offering %q:\n%s", setupHint, s)
	}

	h.Press("2", "enter")
	if !strings.Contains(h.Screen(), "INPUT") {
		t.Fatalf("no site prompt:\n%s", h.Screen())
	}
	h.Type("acme.atlassian.net")
	h.Press("enter")
	if !strings.Contains(h.Screen(), "INPUT") {
		t.Fatalf("no email prompt:\n%s", h.Screen())
	}
	h.Type("oron@acme.io")
	h.Press("enter")
	if got := h.Opener.Opened; len(got) != 1 || got[0] != cloudTokenPage {
		t.Fatalf("opened %v, want Atlassian's API token page", got)
	}
	for i, r := range apiToken {
		h.Type(string(r))
		if typed := apiToken[:i+1]; len(typed) >= 4 && strings.Contains(h.Screen(), typed) {
			t.Fatalf("the typed token shows:\n%s", h.Screen())
		}
	}
	h.Press("enter")

	if got, err := h.Keyring.Get("ghud", ghudAccount); err != nil || got != apiToken {
		t.Fatalf("keyring holds %q (%v), want the token", got, err)
	}
	want := `# my dashboard
theme = "ghud"   # keep it

[[widget]]
type = "clock"

# work queue
# [[widget]]
# type = "jira"
# title = "Work"
# jql = "project = OPS"

[[widget]]
type = "jira"
title = "Work"
jql = "project = OPS"
site = "acme.atlassian.net"
email = "oron@acme.io"
`
	if got := configText(t, h); got != want {
		t.Errorf("config.toml:\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(configText(t, h), apiToken) {
		t.Error("the token reached config.toml")
	}
	s := j.searches()
	if len(s) == 0 || s[len(s)-1].Auth != basic("oron@acme.io", apiToken) {
		t.Fatalf("searches %v, want one with the new token", s)
	}
	if !strings.Contains(h.Screen(), "Work 5") {
		t.Errorf("the Widget didn't load:\n%s", h.Screen())
	}
}

func TestAnExpiredTokenOffersSetUpJiraAgainAndANewTokenLoads(t *testing.T) {
	j := queue(t)
	j.reply = func(r request) (int, http.Header, string) {
		if r.Auth == basic("oron@acme.io", "old-token") {
			return http.StatusUnauthorized, nil, `{"errorMessages":["Client must be authenticated to access this resource."]}`
		}
		return 0, nil, ""
	}
	h := withJira(t, j, hosttest.Options{Width: 100, Height: 10, Env: map[string]string{},
		Keyring: keyring(t, map[string]string{"ghud " + ghudAccount: "old-token"})})
	if s := h.Screen(); !strings.Contains(s, "needs setup") || !strings.Contains(s, setupHint) {
		t.Fatalf("want needs setup offering %q:\n%s", setupHint, s)
	}
	before := configText(t, h)

	h.Press("1", "enter")
	if !strings.Contains(h.Screen(), "› acme.atlassian.net") {
		t.Fatalf("the site prompt isn't prefilled:\n%s", h.Screen())
	}
	h.Press("enter")
	if !strings.Contains(h.Screen(), "› oron@acme.io") {
		t.Fatalf("the email prompt isn't prefilled:\n%s", h.Screen())
	}
	h.Press("enter")
	h.Type("new-token")
	h.Press("enter")

	if got, _ := h.Keyring.Get("ghud", ghudAccount); got != "new-token" {
		t.Fatalf("keyring holds %q, want the new token", got)
	}
	if got := configText(t, h); got != before {
		t.Errorf("config.toml changed with the same site and email:\n%s", got)
	}
	if !strings.Contains(h.Line(0), "Jira 5") {
		t.Errorf("the Widget didn't load with the new token:\n%s", h.Screen())
	}
}
