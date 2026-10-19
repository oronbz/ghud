package jira

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/oronbz/ghud/internal/paths"
	"github.com/oronbz/ghud/internal/ports"
)

// service is the keyring service ghud keeps its own credentials under; the
// account is the Widget's Credential.
const service = "ghud"

// cliService is jira-cli's keychain service; its account is the login.
// ghud reads it, never writes it.
const cliService = "jira-cli"

// token finds an API token for email on site without prompting: from
// JIRA_API_TOKEN, ghud's keyring item, jira-cli's keychain item, then
// ~/.netrc. It's read afresh on every call and never kept.
func token(p ports.Ports, site, email, account string) (string, bool) {
	getenv := p.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	if t := strings.TrimSpace(getenv("JIRA_API_TOKEN")); t != "" {
		return t, true
	}
	if p.Keyring != nil {
		for _, item := range [][2]string{{service, account}, {cliService, email}} {
			if item[1] == "" {
				continue
			}
			if t, err := p.Keyring.Get(item[0], item[1]); err == nil && strings.TrimSpace(t) != "" {
				return strings.TrimSpace(t), true
			}
		}
	}
	home := paths.Home(getenv)
	if home == "" {
		return "", false
	}
	for _, name := range []string{".netrc", "_netrc"} {
		if b, err := os.ReadFile(filepath.Join(home, name)); err == nil {
			if t := netrcPassword(string(b), site, email); t != "" {
				return t, true
			}
		}
	}
	return "", false
}

// netrcPassword is the password of the .netrc entry for machine and login.
// An entry without a login matches any login.
func netrcPassword(text, machine, login string) string {
	var match bool
	var entryLogin, password string
	found := func() bool { return match && password != "" && (entryLogin == "" || entryLogin == login) }
	f := strings.Fields(text)
	for i := 0; i < len(f); i++ {
		next := func() string {
			if i+1 < len(f) {
				i++
				return f[i]
			}
			return ""
		}
		switch f[i] {
		case "machine", "default":
			if found() {
				return password
			}
			// A default entry isn't for Jira, so its password never goes there.
			match, entryLogin, password = false, "", ""
			if f[i] == "machine" {
				match = strings.EqualFold(next(), machine)
			}
		case "login":
			entryLogin = next()
		case "password":
			password = next()
		case "account":
			next()
		case "macdef":
			// A macro runs to the next blank line, which Fields can't see; stop here.
			if found() {
				return password
			}
			return ""
		}
	}
	if found() {
		return password
	}
	return ""
}
