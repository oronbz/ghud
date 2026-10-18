// Package github finds the GitHub token the user's own tools use.
package github

import (
	"context"
	"strings"

	"github.com/oronbz/ghud/internal/ports"
)

// Token borrows the token from GH_TOKEN, GITHUB_TOKEN or `gh auth token`, in
// that order, the way gh itself does; from names where it came from. Both
// are empty when there's none.
func Token(ctx context.Context, getenv func(string) string, c ports.Commands) (tok, from string) {
	for _, k := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if getenv == nil {
			break
		}
		if t := strings.TrimSpace(getenv(k)); t != "" {
			return t, k
		}
	}
	if c == nil {
		return "", ""
	}
	if out, err := c.Output(ctx, "gh", "auth", "token"); err == nil {
		if t := strings.TrimSpace(string(out)); t != "" {
			return t, "`gh auth token`"
		}
	}
	return "", ""
}
