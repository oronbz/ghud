// Package jira is the Jira Widget type: the issues a JQL search finds, by
// default my unresolved assigned ones, with transitions from a choice menu.
package jira

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/oronbz/ghud/internal/ports"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

// DefaultJQL is my unresolved assigned issues, most recently updated first.
const DefaultJQL = "assignee = currentUser() AND statusCategory != Done ORDER BY updated DESC"

var Type = widget.Type{
	Name:        "jira",
	Title:       "Jira",
	Description: "Jira issues from a JQL search, by default your unresolved assigned ones.",
	Settings: []widget.Setting{
		{
			Name: "site", Kind: widget.String, Default: "",
			Description: "Your Jira site, such as \"acme.atlassian.net\".",
			Why:         "Jira needs it; `ghud auth jira` fills it in.",
		},
		{
			Name: "email", Kind: widget.String, Default: "",
			Description: "The email you sign in to Jira with.",
			Why:         "With an API token it signs ghud in as you.",
		},
		{
			Name: "jql", Kind: widget.String, Default: DefaultJQL,
			Description: "The JQL search whose issues the Widget lists.",
			Why:         "Change it to watch another queue, such as your team's board.",
		},
	},
	Refresh: 2 * time.Minute,
	Floor:   30 * time.Second,
	Fetches: true,
	New:     newJira,
}

func newJira(c widget.Config) widget.Widget {
	return jira{
		ports: c.Ports,
		site:  host(c.Settings.String("site")),
		email: strings.TrimSpace(c.Settings.String("email")),
		jql:   c.Settings.String("jql"),
	}
}

type jira struct {
	ports ports.Ports
	// site is the site's host name, such as acme.atlassian.net.
	site, email, jql string
}

// host is a site setting's host name, whether or not it was given as a URL.
func host(site string) string {
	site = strings.TrimSpace(site)
	if u, err := url.Parse(site); err == nil && u.Host != "" {
		return u.Host
	}
	return strings.TrimSuffix(site, "/")
}

// Credential is the account of ghud's keyring item, jira:<site>:<email>,
// which also names the token every Widget on that site and account shares.
func (w jira) Credential() string { return "jira:" + w.site + ":" + w.email }

// browse is the issue's page on the site.
func (w jira) browse(key string) string { return "https://" + w.site + "/browse/" + key }

func (w jira) client(ctx context.Context, now time.Time) (client, error) {
	if w.site == "" || w.email == "" {
		return client{}, needsSetup("Set `site` and `email` for this Jira Widget")
	}
	tok, ok := token(w.ports, w.site, w.email, w.Credential())
	if !ok {
		return client{}, needsSetup("No Jira API token: set JIRA_API_TOKEN, or sign in with jira-cli")
	}
	base := "https://" + w.site
	if e := w.ports.Endpoints.Jira; e != "" {
		base = strings.TrimSuffix(e, "/")
	}
	return client{http: w.ports.HTTP, base: base, email: w.email, token: tok, now: now}, nil
}

// needsSetup is the Status of a Widget ghud can't sign in with. The setup
// Action that fixes it is #43's.
func needsSetup(hint string) *widget.SetupError { return &widget.SetupError{Hint: hint} }

// maxPages caps a search, so a runaway cursor can't page forever.
const maxPages = 20

type searchPage struct {
	Issues        []issueJSON
	NextPageToken string
	IsLast        bool
}

type issueJSON struct {
	Key    string
	Fields struct {
		Summary string
		Status  struct {
			Name           string
			StatusCategory struct{ Key string }
		}
		Priority *struct{ Name string }
	}
}

func (w jira) Refresh(ctx context.Context, now time.Time) (widget.Payload, error) {
	c, err := w.client(ctx, now)
	if err != nil {
		return widget.Payload{}, err
	}
	var issues []issueJSON
	cursor := ""
	for range maxPages {
		q := url.Values{"jql": {w.jql}, "fields": {"summary,status,priority"}, "maxResults": {"100"}}
		if cursor != "" {
			q.Set("nextPageToken", cursor)
		}
		var page searchPage
		if err := c.get(ctx, "/rest/api/3/search/jql?"+q.Encode(), &page); err != nil {
			return widget.Payload{}, w.status(err)
		}
		issues = append(issues, page.Issues...)
		if page.IsLast || page.NextPageToken == "" || page.NextPageToken == cursor {
			break
		}
		cursor = page.NextPageToken
	}
	return w.payload(issues), nil
}

// status turns a rejected token into needs setup.
func (w jira) status(err error) error {
	if errors.As(err, new(authError)) {
		return needsSetup(fmt.Sprintf("Jira rejected the API token for %s; check JIRA_API_TOKEN or sign in again", w.email))
	}
	return err
}

func (w jira) payload(issues []issueJSON) widget.Payload {
	p := widget.Payload{
		Columns: []widget.Column{{Priority: 3}, {Priority: 1}, {Priority: 0, Flex: true}, {Priority: 2, Right: true}},
		Empty:   "No issues match the JQL.",
		Count:   len(issues),
	}
	if w.jql == DefaultJQL {
		p.Empty = "Nothing assigned to you."
	}
	for _, is := range issues {
		glyph, tok := priority(is.Fields.Priority)
		p.Items = append(p.Items, widget.Item{
			ID: is.Key,
			Fields: []widget.Field{
				widget.F(tok, glyph),
				widget.F(theme.Dim, is.Key),
				widget.F(theme.Fg, is.Fields.Summary),
				widget.F(category(is.Fields.Status.StatusCategory.Key), is.Fields.Status.Name),
			},
			Actions: []widget.Action{
				{ID: "open", Key: "enter", Label: "open", Primary: true, Verb: widget.Open, Target: w.browse(is.Key)},
				{ID: transitionAction, Key: "t", Label: "transition", Input: widget.ChoiceInput, Prompt: "Transition " + is.Key},
			},
		})
	}
	return p
}

// priority is the glyph for an issue's priority, Cloud's names or Data Center's.
func priority(p *struct{ Name string }) (string, theme.Token) {
	if p == nil {
		return "·", theme.Faint
	}
	switch strings.ToLower(p.Name) {
	case "highest", "blocker", "critical":
		return "⇈", theme.Bad
	case "high", "major":
		return "↑", theme.Bad
	case "medium":
		return "=", theme.Warn
	case "low", "minor":
		return "↓", theme.Info
	case "lowest", "trivial":
		return "⇊", theme.Info
	}
	return "·", theme.Faint
}

// category colours a status by its category: to do, in progress or done.
func category(key string) theme.Token {
	switch key {
	case "indeterminate":
		return theme.Accent
	case "done":
		return theme.Ok
	}
	return theme.Dim
}

const transitionAction = "transition"

type transitionsJSON struct {
	Transitions []struct {
		ID, Name string
		To       struct{ Name string }
		Fields   map[string]struct {
			Required, HasDefaultValue bool
		}
	}
}

// Choices are the issue's transitions; one with a required field can't be
// filled in here, so it opens the issue instead.
func (w jira) Choices(ctx context.Context, inv widget.Invocation) ([]widget.Choice, error) {
	c, err := w.client(ctx, inv.Now)
	if err != nil {
		return nil, err
	}
	var ts transitionsJSON
	if err := c.get(ctx, "/rest/api/3/issue/"+url.PathEscape(inv.Item)+"/transitions?expand=transitions.fields", &ts); err != nil {
		return nil, err
	}
	var out []widget.Choice
	for _, t := range ts.Transitions {
		ch := widget.Choice{ID: t.ID, Label: t.Name}
		if t.To.Name != "" && !strings.EqualFold(t.To.Name, t.Name) {
			ch.Label += " → " + t.To.Name
		}
		for _, f := range t.Fields {
			if f.Required && !f.HasDefaultValue {
				ch.Target = w.browse(inv.Item)
				break
			}
		}
		out = append(out, ch)
	}
	return out, nil
}

func (w jira) Invoke(ctx context.Context, inv widget.Invocation) (widget.Result, error) {
	if inv.Action != transitionAction {
		return widget.Result{}, fmt.Errorf("Jira has no Action %q", inv.Action)
	}
	c, err := w.client(ctx, inv.Now)
	if err != nil {
		return widget.Result{}, err
	}
	body := map[string]any{"transition": map[string]string{"id": inv.Choice}}
	if err := c.send(ctx, http.MethodPost, "/rest/api/3/issue/"+url.PathEscape(inv.Item)+"/transitions", body); err != nil {
		return widget.Result{}, err
	}
	return widget.Result{Toast: "Transitioned " + inv.Item}, nil
}
