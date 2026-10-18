// Package prs is the GitHub PRs Widget type: open pull requests waiting for
// my review, and the ones a teammate already reviewed for my team.
package prs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/oronbz/ghud/internal/ports"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

var Type = widget.Type{
	Name:        "prs",
	Title:       "Review requests",
	Description: "GitHub pull requests waiting for your review.",
	Settings: []widget.Setting{{
		Name: "drafts", Kind: widget.Bool, Default: true,
		Description: "Show draft PRs.",
		Why:         "Set it to false to see only PRs that are ready for review.",
	}},
	Refresh: 2 * time.Minute,
	Floor:   30 * time.Second,
	Fetches: true,
	New:     newPRs,
}

// handled is the group of PRs a teammate already reviewed for my team.
const handled = "Handled by a teammate"

func newPRs(c widget.Config) widget.Widget {
	return prs{ports: c.Ports, drafts: c.Settings.Bool("drafts")}
}

type prs struct {
	ports  ports.Ports
	drafts bool
}

func (prs) Credential() string { return credential }

func (w prs) client(ctx context.Context, now time.Time) (client, error) {
	tok, err := token(ctx, w.ports)
	if err != nil {
		return client{}, err
	}
	return client{http: w.ports.HTTP, endpoints: w.ports.Endpoints, token: tok, now: now}, nil
}

// query fetches the viewer and both searches in one request.
const query = `query($requested: String!, $involved: String!) {
  viewer { login }
  requested: search(query: $requested, type: ISSUE, first: 50) { nodes { ...pr } }
  involved: search(query: $involved, type: ISSUE, first: 50) { nodes { ...pr } }
}
fragment pr on PullRequest {
  number title url createdAt additions deletions headRefName
  repository { nameWithOwner name }
  author { login }
  commits(last: 1) { nodes { commit { statusCheckRollup { state } } } }
  reviewRequests(first: 20) {
    nodes { asCodeOwner requestedReviewer { __typename ... on User { login } ... on Team { slug } } }
  }
  latestReviews(first: 20) { nodes { author { login } onBehalfOf(first: 5) { nodes { slug } } } }
}`

type searchData struct {
	Viewer    struct{ Login string }
	Requested struct{ Nodes []node }
	Involved  struct{ Nodes []node }
}

type node struct {
	Number      int
	Title       string
	URL         string
	CreatedAt   time.Time
	Additions   int
	Deletions   int
	HeadRefName string
	Repository  struct{ NameWithOwner, Name string }
	Author      struct{ Login string }
	Commits     struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct{ State string }
			}
		}
	}
	ReviewRequests struct {
		Nodes []struct {
			AsCodeOwner       bool
			RequestedReviewer struct {
				Typename string `json:"__typename"`
				Login    string
				Slug     string
			}
		}
	}
	LatestReviews struct {
		Nodes []struct {
			Author     struct{ Login string }
			OnBehalfOf struct{ Nodes []struct{ Slug string } }
		}
	}
}

func (w prs) Refresh(ctx context.Context, now time.Time) (widget.Payload, error) {
	c, err := w.client(ctx, now)
	if errors.Is(err, errNoToken) {
		return widget.Payload{}, needsToken()
	}
	if err != nil {
		return widget.Payload{}, err
	}
	filter := "is:pr is:open archived:false"
	if !w.drafts {
		filter += " draft:false"
	}
	var d searchData
	err = c.graphQL(ctx, query, map[string]any{
		"requested": filter + " review-requested:@me",
		"involved":  filter + " review-involves:@me",
	}, &d)
	if err != nil {
		return widget.Payload{}, err
	}
	return payload(d, now), nil
}

func payload(d searchData, now time.Time) widget.Payload {
	p := widget.Payload{
		Columns: []widget.Column{
			{Priority: 0}, {Priority: 1}, {Priority: 2, Flex: true}, {Priority: 3, Right: true},
			{Priority: 4}, {Priority: 5}, {Priority: 6, Right: true},
		},
		Empty: "No reviews waiting on you.",
	}
	me := d.Viewer.Login
	seen := map[string]bool{}
	for _, n := range d.Requested.Nodes {
		if n.Number == 0 || seen[n.id()] {
			continue
		}
		seen[n.id()] = true
		p.Items = append(p.Items, n.item("", reason(n, me), now, me))
	}
	p.Count = len(p.Items)
	for _, n := range d.Involved.Nodes {
		if n.Number == 0 || seen[n.id()] || n.Author.Login == me {
			continue
		}
		who := teammates(n, me)
		if len(who) == 0 {
			continue
		}
		seen[n.id()] = true
		p.Items = append(p.Items, n.item(handled, []widget.Span{{Text: "✓ " + strings.Join(who, ", "), Token: theme.Ok}}, now, me))
	}
	return p
}

// id is the PR's Item id, owner/repo#number.
func (n node) id() string { return n.Repository.NameWithOwner + "#" + strconv.Itoa(n.Number) }

func (n node) item(group string, why widget.Field, now time.Time, me string) widget.Item {
	glyph, tok := ci(n)
	it := widget.Item{
		ID:    n.id(),
		Group: group,
		Fields: []widget.Field{
			widget.F(tok, glyph),
			widget.F(theme.Dim, n.Repository.Name+"#"+strconv.Itoa(n.Number)),
			widget.F(theme.Fg, n.Title),
			{{Text: "+" + strconv.Itoa(n.Additions), Token: theme.Ok}, {Text: " −" + strconv.Itoa(n.Deletions), Token: theme.Bad}},
			widget.F(theme.Dim, n.Author.Login),
			why,
			widget.F(theme.Faint, age(now.Sub(n.CreatedAt))),
		},
		Actions: []widget.Action{
			{ID: "open", Key: "enter", Label: "open", Primary: true, Verb: widget.Open, Target: n.URL},
			{ID: "approve", Key: "a", Label: "approve", Confirm: true},
		},
	}
	if r := n.removal(me); r != "" {
		it.Actions = append(it.Actions, widget.Action{ID: "remove", Key: "r", Label: "remove me", Confirm: true, Value: r})
	}
	it.Actions = append(it.Actions, widget.Action{ID: "copy", Key: "y", Label: "copy branch", Verb: widget.Copy, Target: n.HeadRefName})
	return it
}

// ci is the glyph for the head commit's combined checks.
func ci(n node) (string, theme.Token) {
	state := ""
	if c := n.Commits.Nodes; len(c) > 0 && c[0].Commit.StatusCheckRollup != nil {
		state = c[0].Commit.StatusCheckRollup.State
	}
	switch state {
	case "SUCCESS":
		return "✓", theme.Ok
	case "FAILURE", "ERROR":
		return "✗", theme.Bad
	case "PENDING", "EXPECTED":
		return "●", theme.Warn
	}
	return "·", theme.Faint
}

// reason is why I was asked to review: @you, a team of mine, or as a code owner.
func reason(n node, me string) widget.Field {
	var team string
	owner := false
	for _, r := range n.ReviewRequests.Nodes {
		rv := r.RequestedReviewer
		switch {
		case rv.Typename == "User" && rv.Login == me:
			if r.AsCodeOwner {
				return widget.F(theme.Alt, "owner")
			}
			return widget.F(theme.Accent, "@you")
		case rv.Typename == "Team" && team == "" && !owner:
			team, owner = rv.Slug, r.AsCodeOwner
		}
	}
	if owner {
		return widget.F(theme.Alt, "owner")
	}
	return widget.F(theme.Info, team)
}

// removal is the review request "remove me" withdraws: "user:<login>" when
// I was asked directly, else "team:<slug>" for the team I was asked through.
// It's carried in the Action's Value so a cached payload can still run it.
func (n node) removal(me string) string {
	var team string
	for _, r := range n.ReviewRequests.Nodes {
		rv := r.RequestedReviewer
		if rv.Typename == "User" && rv.Login == me {
			return "user:" + me
		}
		if rv.Typename == "Team" && team == "" {
			team = "team:" + rv.Slug
		}
	}
	return team
}

// teammates are those who reviewed the PR for a team, unless I reviewed it myself.
func teammates(n node, me string) []string {
	var who []string
	for _, r := range n.LatestReviews.Nodes {
		if r.Author.Login == me {
			return nil
		}
		if len(r.OnBehalfOf.Nodes) > 0 && !slices.Contains(who, r.Author.Login) {
			who = append(who, r.Author.Login)
		}
	}
	return who
}

// age is how long ago something happened in its largest whole unit: 45m, 3h, 2d.
func age(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", max(0, int(d.Minutes())))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func (w prs) Invoke(ctx context.Context, inv widget.Invocation) (widget.Result, error) {
	repo, num, ok := strings.Cut(inv.Item, "#")
	if !ok {
		return widget.Result{}, fmt.Errorf("PRs has no Item %q", inv.Item)
	}
	c, err := w.client(ctx, inv.Now)
	if errors.Is(err, errNoToken) {
		return widget.Result{}, errors.New("no GitHub token; run `gh auth login`")
	}
	if err != nil {
		return widget.Result{}, err
	}
	pull := "/repos/" + repo + "/pulls/" + num
	short := repo[strings.Index(repo, "/")+1:] + "#" + num
	switch inv.Action {
	case "approve":
		if err := c.rest(ctx, http.MethodPost, pull+"/reviews", map[string]any{"event": "APPROVE"}); err != nil {
			return widget.Result{}, err
		}
		return widget.Result{Toast: "Approved " + short}, nil
	case "remove":
		body := map[string]any{"reviewers": []string{}, "team_reviewers": []string{}}
		kind, who, _ := strings.Cut(inv.Base, ":")
		switch kind {
		case "user":
			body["reviewers"] = []string{who}
		case "team":
			body["team_reviewers"] = []string{who}
		default:
			return widget.Result{}, errors.New("you aren't a requested reviewer of " + short)
		}
		if err := c.rest(ctx, http.MethodDelete, pull+"/requested_reviewers", body); err != nil {
			return widget.Result{}, err
		}
		return widget.Result{Toast: "Removed you from the reviewers of " + short}, nil
	}
	return widget.Result{}, fmt.Errorf("PRs has no Action %q", inv.Action)
}
