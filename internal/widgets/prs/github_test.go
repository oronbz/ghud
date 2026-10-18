package prs_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// github is a fake GitHub: GraphQL search over fixture PRs, and the REST
// endpoints the PRs Widget's Actions call. It records every request.
type github struct {
	srv *httptest.Server

	mu        sync.Mutex
	viewer    string
	requested []pr
	involved  []pr
	requests  []request
	// reply, when set, answers a request instead of the fixtures; status 0 falls through.
	reply func(r request) (status int, header http.Header, body string)
}

type request struct {
	Method, Path, Auth string
	Body               string
}

func (r request) String() string { return r.Method + " " + r.Path }

type pr struct {
	repo    string
	number  int
	title   string
	author  string
	branch  string
	adds    int
	dels    int
	ci      string // SUCCESS, FAILURE, PENDING or "" for no checks
	created time.Time
	// requests are the PR's pending review requests: a login, or "team:<slug>";
	// a trailing "!" marks a code owner request.
	requests []string
	// reviews are latest reviews: "<login>" or "<login> for <team slug>".
	reviews []string
}

func newGitHub(t *testing.T) *github {
	g := &github{viewer: "oronbz"}
	g.srv = httptest.NewServer(http.HandlerFunc(g.serve))
	t.Cleanup(g.srv.Close)
	return g
}

func (g *github) serve(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	req := request{Method: r.Method, Path: r.URL.Path, Auth: r.Header.Get("Authorization"), Body: string(b)}
	g.mu.Lock()
	g.requests = append(g.requests, req)
	reply := g.reply
	g.mu.Unlock()
	if reply != nil {
		if status, header, body := reply(req); status != 0 {
			for k, v := range header {
				w.Header()[k] = v
			}
			w.WriteHeader(status)
			io.WriteString(w, body)
			return
		}
	}
	if r.Method == http.MethodPost && r.URL.Path == "/graphql" {
		g.search(w, b)
		return
	}
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"id": 1, "state": "APPROVED"}`)
	case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/requested_reviewers"):
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{}`)
	default:
		http.NotFound(w, r)
	}
}

// search answers the Widget's one GraphQL request: the viewer, plus each
// search whose query string asks for review requests or review involvement.
func (g *github) search(w http.ResponseWriter, body []byte) {
	var q struct {
		Variables map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(body, &q); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	data := map[string]any{"viewer": map[string]any{"login": g.viewer}}
	for name, v := range q.Variables {
		s, _ := v.(string)
		switch {
		case strings.Contains(s, "review-requested:@me"):
			data[name] = results(g.requested)
		case strings.Contains(s, "review-involves:@me"):
			data[name] = results(g.involved)
		}
	}
	json.NewEncoder(w).Encode(map[string]any{"data": data})
}

func results(prs []pr) map[string]any {
	nodes := []any{}
	for _, p := range prs {
		nodes = append(nodes, p.node())
	}
	return map[string]any{"issueCount": len(nodes), "nodes": nodes}
}

func (p pr) node() map[string]any {
	var rollup any
	if p.ci != "" {
		rollup = map[string]any{"state": p.ci}
	}
	var requests []any
	for _, r := range p.requests {
		owner := strings.HasSuffix(r, "!")
		r = strings.TrimSuffix(r, "!")
		reviewer := map[string]any{"__typename": "User", "login": r}
		if slug, ok := strings.CutPrefix(r, "team:"); ok {
			reviewer = map[string]any{"__typename": "Team", "slug": slug, "name": slug}
		}
		requests = append(requests, map[string]any{"asCodeOwner": owner, "requestedReviewer": reviewer})
	}
	var reviews []any
	for _, r := range p.reviews {
		login, team, _ := strings.Cut(r, " for ")
		var teams []any
		if team != "" {
			teams = append(teams, map[string]any{"slug": team, "name": team})
		}
		reviews = append(reviews, map[string]any{
			"author": map[string]any{"login": login}, "state": "APPROVED",
			"onBehalfOf": map[string]any{"nodes": teams},
		})
	}
	return map[string]any{
		"number": p.number, "title": p.title, "isDraft": false,
		"url":       "https://github.com/" + p.repo + "/pull/" + itoa(p.number),
		"createdAt": p.created.Format(time.RFC3339),
		"additions": p.adds, "deletions": p.dels, "headRefName": p.branch,
		"repository":     map[string]any{"nameWithOwner": p.repo, "name": p.repo[strings.Index(p.repo, "/")+1:]},
		"author":         map[string]any{"login": p.author},
		"commits":        map[string]any{"nodes": []any{map[string]any{"commit": map[string]any{"statusCheckRollup": rollup}}}},
		"reviewRequests": map[string]any{"nodes": requests},
		"latestReviews":  map[string]any{"nodes": reviews},
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// Requests are the requests GitHub has received so far.
func (g *github) Requests() []request {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]request(nil), g.requests...)
}

// searches counts the GraphQL requests so far.
func (g *github) searches() int {
	n := 0
	for _, r := range g.Requests() {
		if r.Path == "/graphql" {
			n++
		}
	}
	return n
}
