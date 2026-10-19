package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oronbz/visor/internal/cli"
	"github.com/oronbz/visor/internal/hosttest"
)

// authJira runs `visor auth jira` in a temp home holding config (none when
// empty), answering its prompts with stdin.
func authJira(t *testing.T, config, stdin string) (code int, home string, k *hosttest.Keyring, o *hosttest.Opener, stdout, stderr *bytes.Buffer) {
	t.Helper()
	home = t.TempDir()
	if config != "" {
		dir := filepath.Join(home, ".config", "visor")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	getenv := func(k string) string { return map[string]string{"HOME": home}[k] }
	pp, _ := hermetic(t, getenv)
	k, o = pp.Keyring.(*hosttest.Keyring), pp.Opener.(*hosttest.Opener)
	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	code = run(t, cli.System{Args: []string{"auth", "jira"}, Getenv: getenv, Ports: pp,
		Stdin: strings.NewReader(stdin), Stdout: stdout, Stderr: stderr})
	return code, home, k, o, stdout, stderr
}

func configOf(t *testing.T, home string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(home, ".config", "visor", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAuthJiraStoresTheTokenInTheKeyringAndTheSiteAndEmailInTheConfig(t *testing.T) {
	config := `# mine
mouse = false

[[widget]]
type = "jira"
id = "work"   # my queue
jql = "project = OPS"
`
	code, home, k, o, stdout, stderr := authJira(t, config, "acme.atlassian.net\noron@acme.io\nsecret-token\n")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	if got, err := k.Get("visor", "jira:acme.atlassian.net:oron@acme.io"); err != nil || got != "secret-token" {
		t.Errorf("keyring holds %q (%v), want the token", got, err)
	}
	want := `# mine
mouse = false

# [[widget]]
# type = "jira"
# id = "work"   # my queue
# jql = "project = OPS"

[[widget]]
type = "jira"
id = "work"   # my queue
jql = "project = OPS"
site = "acme.atlassian.net"
email = "oron@acme.io"
`
	if got := configOf(t, home); got != want {
		t.Errorf("config.toml:\n%s\nwant:\n%s", got, want)
	}
	if got := o.Opened; len(got) != 1 || got[0] != "https://id.atlassian.com/manage-profile/security/api-tokens" {
		t.Errorf("opened %v, want Atlassian's API token page", got)
	}
	out := stdout.String() + stderr.String()
	if strings.Contains(out, "secret-token") {
		t.Errorf("the token was echoed:\n%s", out)
	}
	for _, want := range []string{"Jira site", "Email", "API token", "Saved the Jira token to the keyring"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestAuthJiraWithNoJiraWidgetAppendsAFilledBlock(t *testing.T) {
	config := "[[widget]]\ntype = \"clock\"\n"
	code, home, _, _, _, stderr := authJira(t, config, "jira.acme.internal\n\ndc-pat\n")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	want := config + "\n[[widget]]\ntype = \"jira\"\nid = \"jira\"\nsite = \"jira.acme.internal\"\nemail = \"\"\n"
	if got := configOf(t, home); got != want {
		t.Errorf("config.toml:\n%s\nwant:\n%s", got, want)
	}
}

func TestAuthJiraWithNoConfigWritesTheDefaultWidgetsWithJiraFilledIn(t *testing.T) {
	code, home, _, _, _, stderr := authJira(t, "", "acme.atlassian.net\noron@acme.io\nsecret-token\n")
	if code != 0 {
		t.Fatalf("exit %d, stderr: %s", code, stderr)
	}
	got := configOf(t, home)
	if !strings.HasSuffix(got, "[[widget]]\ntype = \"jira\"\nid = \"jira\"\nsite = \"acme.atlassian.net\"\nemail = \"oron@acme.io\"\n") {
		t.Errorf("config.toml doesn't end in the filled Jira block:\n%s", got)
	}
	for _, typ := range []string{"clock", "tasks", "prs"} {
		if !strings.Contains(got, "[[widget]]\ntype = \""+typ+"\"\n") {
			t.Errorf("config.toml lacks the default %s Widget:\n%s", typ, got)
		}
	}
}

func TestAuthJiraWithoutATokenChangesNothing(t *testing.T) {
	config := "[[widget]]\ntype = \"jira\"\n"
	code, home, k, _, _, stderr := authJira(t, config, "acme.atlassian.net\noron@acme.io\n")
	if code == 0 {
		t.Fatal("exit 0 with no token")
	}
	if !strings.Contains(stderr.String(), "no token") {
		t.Errorf("stderr %q doesn't say why", stderr)
	}
	if got := configOf(t, home); got != config {
		t.Errorf("config.toml changed:\n%s", got)
	}
	if _, err := k.Get("visor", "jira:acme.atlassian.net:oron@acme.io"); err == nil {
		t.Error("a token reached the keyring")
	}
}
