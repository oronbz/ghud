package prs

import (
	"context"
	"errors"
	"strings"

	"github.com/oronbz/ghud/internal/ports"
	"github.com/oronbz/ghud/internal/widget"
)

// credential names the GitHub token every PRs Widget shares, so a rate limit
// on one pauses them all.
const credential = "github:github.com"

var errNoToken = errors.New("no GitHub token")

// token borrows a GitHub token from GH_TOKEN, GITHUB_TOKEN or `gh auth token`.
// It's read afresh each time and never stored, so a `gh auth login` is
// picked up on the next refresh.
func token(ctx context.Context, p ports.Ports) (string, error) {
	if p.Getenv != nil {
		for _, k := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
			if t := strings.TrimSpace(p.Getenv(k)); t != "" {
				return t, nil
			}
		}
	}
	if out, err := p.Commands.Output(ctx, "gh", "auth", "token"); err == nil {
		if t := strings.TrimSpace(string(out)); t != "" {
			return t, nil
		}
	}
	return "", errNoToken
}

// needsToken is the Status of a Widget with no token to borrow. #41 adds
// the paste-a-classic-token setup Action here, and ghud's keyring item to token.
func needsToken() *widget.SetupError {
	return &widget.SetupError{Hint: "Run `gh auth login`"}
}
