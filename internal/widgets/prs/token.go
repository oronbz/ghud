package prs

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/oronbz/ghud/internal/ports"
	"github.com/oronbz/ghud/internal/widget"
)

// credential names the GitHub token every PRs Widget shares, so a rate limit
// on one pauses them all. It's also the account of ghud's keyring item.
const credential = "github:github.com"

// service is the keyring service ghud keeps its own credentials under.
const service = "ghud"

// tokenPage creates a classic token with repo scope.
const tokenPage = "https://github.com/settings/tokens/new?scopes=repo&description=ghud"

// pasteToken is the setup Action that stores a pasted classic token.
const pasteToken = "paste-token"

var (
	errNoToken   = errors.New("no GitHub token")
	errNoKeyring = errors.New("no GitHub token and no keyring")
)

// token borrows a GitHub token from GH_TOKEN, GITHUB_TOKEN or `gh auth token`,
// else reads the one pasted into ghud from the keyring; stored says which.
// It's read afresh each time and never kept, so a `gh auth login` is picked
// up on the next refresh.
func token(ctx context.Context, p ports.Ports) (tok string, stored bool, err error) {
	if p.Getenv != nil {
		for _, k := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
			if t := strings.TrimSpace(p.Getenv(k)); t != "" {
				return t, false, nil
			}
		}
	}
	if out, err := p.Commands.Output(ctx, "gh", "auth", "token"); err == nil {
		if t := strings.TrimSpace(string(out)); t != "" {
			return t, false, nil
		}
	}
	if p.Keyring == nil {
		return "", false, errNoKeyring
	}
	t, err := p.Keyring.Get(service, credential)
	switch {
	case errors.Is(err, ports.ErrSecretNotFound):
		return "", false, errNoToken
	case err != nil:
		return "", false, errNoKeyring
	case strings.TrimSpace(t) == "":
		return "", false, errNoToken
	}
	return strings.TrimSpace(t), true, nil
}

// needsToken is the Status of a Widget with no token to use, err being why.
func needsToken(err error) *widget.SetupError {
	if errors.Is(err, errNoKeyring) {
		return &widget.SetupError{Hint: "Set GH_TOKEN, or run `gh auth login` (no keyring to keep a token in)"}
	}
	return &widget.SetupError{Hint: "Run `gh auth login`, or press Enter to paste a classic token", Action: pasteAction()}
}

// rejected is the Status of a Widget whose stored token GitHub turned down.
func rejected() *widget.SetupError {
	return &widget.SetupError{
		Hint:   "GitHub rejected the saved token. Run `gh auth login`, or press Enter to paste a classic token",
		Action: pasteAction(),
	}
}

func pasteAction() *widget.Action {
	return &widget.Action{
		ID: pasteToken, Label: "paste token", Input: widget.SecretInput,
		Prompt: "Paste a classic token with repo scope", Target: tokenPage,
	}
}

// store keeps a pasted classic token in the keyring.
func store(p ports.Ports, text string) (widget.Result, error) {
	tok := strings.TrimSpace(text)
	switch {
	case tok == "":
		return widget.Result{}, errors.New("no token pasted")
	case strings.HasPrefix(tok, "github_pat_"):
		return widget.Result{}, errors.New("Use a classic token with repo scope")
	case p.Keyring == nil:
		return widget.Result{}, errors.New("no keyring to keep the token in; set GH_TOKEN instead")
	}
	if err := p.Keyring.Set(service, credential, tok); err != nil {
		return widget.Result{}, fmt.Errorf("couldn't keep the token in the keyring: %w", err)
	}
	return widget.Result{Toast: "Saved the GitHub token to the keyring"}, nil
}
