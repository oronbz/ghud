package jira_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/hosttest"
)

func TestAnEmptyEmailSendsTheTokenAsADataCenterBearerPAT(t *testing.T) {
	j := queue(t)
	h := withJira(t, j, hosttest.Options{Config: "[[widget]]\ntype = \"jira\"\nsite = \"jira.acme.internal\"\n",
		Env: map[string]string{"JIRA_API_TOKEN": "dc-pat"}})
	s := j.searches()
	if len(s) == 0 {
		t.Fatalf("Jira wasn't searched:\n%s", h.Screen())
	}
	if got, want := s[0].Auth, "Bearer dc-pat"; got != want {
		t.Errorf("Authorization %q, want %q", got, want)
	}
}

// paths are the requests' paths, in order.
func paths(rs []request) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Method+" "+r.Path)
	}
	return out
}

func TestAScopedTokenSwitchesToTheAtlassianAPIRouteOnceAndKeepsIt(t *testing.T) {
	j := queue(t)
	j.page = 50
	j.scoped = "scoped-token"
	h := withJira(t, j, hosttest.Options{Env: map[string]string{"JIRA_API_TOKEN": "scoped-token"}})

	want := []string{
		"GET /rest/api/3/search/jql",
		"GET /_edge/tenant_info",
		"GET " + apiRoute + "/rest/api/3/search/jql",
	}
	if got := paths(j.Requests()); !slices.Equal(got, want) {
		t.Fatalf("requests %q, want %q", got, want)
	}
	if got := j.Requests()[2].Auth; got != basic("oron@acme.io", "scoped-token") {
		t.Errorf("api.atlassian.com got Authorization %q", got)
	}
	if !strings.Contains(h.Line(0), "Jira 5") {
		t.Fatalf("the Widget didn't load:\n%s", h.Screen())
	}

	h.Press("ctrl+r")
	want = append(want, "GET "+apiRoute+"/rest/api/3/search/jql")
	if got := paths(j.Requests()); !slices.Equal(got, want) {
		t.Errorf("after a refresh requests %q, want %q", got, want)
	}

	h.Press("1", "t")
	if got := paths(j.Requests())[len(want)]; got != "GET "+apiRoute+"/rest/api/3/issue/OPS-12/transitions" {
		t.Errorf("transitions fetched with %q, want the api.atlassian.com route", got)
	}
}

func TestATokenBothRoutesTurnDownNeedsSetup(t *testing.T) {
	j := queue(t)
	j.reply = func(r request) (int, http.Header, string) {
		if r.Path == "/_edge/tenant_info" {
			return 0, nil, ""
		}
		return http.StatusUnauthorized, nil, `{"errorMessages":["Client must be authenticated to access this resource."]}`
	}
	h := withJira(t, j, hosttest.Options{Width: 100})
	if s := h.Screen(); !strings.Contains(s, "Jira rejected the API token for oron@acme.io") || !strings.Contains(s, setupHint) {
		t.Errorf("want needs setup offering %q:\n%s", setupHint, s)
	}
	if n := len(j.searches()); n != 2 {
		t.Errorf("searched %d times, want the site then the api.atlassian.com route", n)
	}
}
