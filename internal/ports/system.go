package ports

import (
	"context"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/zalando/go-keyring"
)

// ErrSecretNotFound is what Keyring.Get and Delete return for a missing secret.
var ErrSecretNotFound = keyring.ErrNotFound

// System returns the real adapters. getenv is the process environment.
func System(getenv func(string) string) Ports {
	return Ports{
		Clock:     systemClock{},
		Opener:    systemOpener{},
		Clipboard: osc52{},
		Editor:    execEditor{getenv: getenv},
		Keyring:   osKeyring{},
		Commands:  execCommands{},
		EventKit:  noEventKit{},
		Machine:   newMachine(),
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		Endpoints: DefaultEndpoints(),
	}
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

func (systemClock) After(d time.Duration, msg func(time.Time) tea.Msg) tea.Cmd {
	return tea.Tick(d, msg)
}

type systemOpener struct{}

func (systemOpener) Open(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start()
}

type osc52 struct{}

func (osc52) Copy(text string) tea.Cmd { return tea.SetClipboard(text) }

type execEditor struct{ getenv func(string) string }

func (e execEditor) Edit(path string, done func(error) tea.Msg) tea.Cmd {
	return tea.ExecProcess(editorCommand(e.getenv, runtime.GOOS, path), func(err error) tea.Msg { return done(err) })
}

// editorCommand opens path in $VISUAL, else $EDITOR, else vi.
func editorCommand(getenv func(string) string, goos, path string) *exec.Cmd {
	editor := getenv("VISUAL")
	if editor == "" {
		editor = getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	if goos == "windows" {
		return exec.Command(editor, path)
	}
	// Through the shell so that EDITOR="code -w" works; the path stays out of the script as $0.
	return exec.Command("sh", "-c", editor+` "$0"`, path)
}

type osKeyring struct{}

func (osKeyring) Get(service, account string) (string, error) { return keyring.Get(service, account) }
func (osKeyring) Set(service, account, secret string) error {
	return keyring.Set(service, account, secret)
}
func (osKeyring) Delete(service, account string) error { return keyring.Delete(service, account) }

type execCommands struct{}

func (execCommands) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

type noEventKit struct{}

func (noEventKit) Status(Entity) Access { return AccessUnsupported }

func (noEventKit) Request(context.Context, Entity) (Access, error) { return AccessUnsupported, nil }
