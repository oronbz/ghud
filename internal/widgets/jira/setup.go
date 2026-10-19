package jira

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/oronbz/visor/internal/ports"
	"github.com/oronbz/visor/internal/widget"
)

// Setup is the one flow behind `visor auth jira` and the Set up Jira
// Action: prompts for the site, the email and, with the page that makes one
// opened, a masked API token. Only the token goes to the keyring; site and
// email come back as the Widget's settings. Each step's answers so far ride
// in the next step's Action ID, so the flow keeps no state between steps.
type Setup struct{ Keyring ports.Keyring }

const (
	setupSite  = "setup"
	setupEmail = "setup-email"
	setupToken = "setup-token"
	setupLabel = "Set up Jira"
)

// cloudTokenPage is where Atlassian Cloud users make API tokens.
const cloudTokenPage = "https://id.atlassian.com/manage-profile/security/api-tokens"

// Start is the flow's first step, its prompts prefilled with site and email.
func (Setup) Start(site, email string) widget.Action {
	return widget.Action{
		ID: step(setupSite, url.Values{"email": {email}}), Label: setupLabel,
		Input: widget.TextInput, Prompt: "Jira site, such as acme.atlassian.net", Value: site,
	}
}

func step(id string, v url.Values) string { return id + "?" + v.Encode() }

// IsSetup says whether an Action ID is one of the flow's steps.
func IsSetup(action string) bool {
	id, _, _ := strings.Cut(action, "?")
	return id == setupSite || id == setupEmail || id == setupToken
}

// Invoke answers one step: it returns the next one, or, after the token,
// the settings to write.
func (s Setup) Invoke(_ context.Context, inv widget.Invocation) (widget.Result, error) {
	id, q, _ := strings.Cut(inv.Action, "?")
	v, _ := url.ParseQuery(q)
	text := strings.TrimSpace(inv.Text)
	switch id {
	case setupSite:
		site := host(text)
		if site == "" {
			return widget.Result{}, errors.New("enter your Jira site, such as acme.atlassian.net")
		}
		return widget.Result{Then: &widget.Action{
			ID: step(setupEmail, url.Values{"site": {site}}), Label: setupLabel, Input: widget.TextInput,
			Prompt: "Email you sign in with (empty for a Data Center token)", Value: v.Get("email"),
		}}, nil
	case setupEmail:
		site, email := v.Get("site"), text
		prompt, page := "API token for "+email, cloudTokenPage
		if email == "" {
			prompt = "Personal access token"
			page = "https://" + site + "/secure/ViewProfile.jspa?selectedTab=com.atlassian.pats.pats-plugin:jira-user-personal-access-tokens"
		}
		return widget.Result{Then: &widget.Action{
			ID: step(setupToken, url.Values{"site": {site}, "email": {email}}), Label: setupLabel,
			Input: widget.SecretInput, Prompt: prompt, Target: page,
		}}, nil
	case setupToken:
		site, email := v.Get("site"), v.Get("email")
		if text == "" {
			return widget.Result{}, errors.New("no token entered")
		}
		if s.Keyring == nil {
			return widget.Result{}, errors.New("no keyring to keep the token in; set JIRA_API_TOKEN instead")
		}
		if err := s.Keyring.Set(service, account(site, email), text); err != nil {
			return widget.Result{}, fmt.Errorf("can't save the token to the keyring (set JIRA_API_TOKEN instead): %w", err)
		}
		return widget.Result{
			Toast:    "Saved the Jira token to the keyring",
			Settings: widget.Settings{"site": site, "email": email},
		}, nil
	}
	return widget.Result{}, fmt.Errorf("Jira has no Action %q", inv.Action)
}
