package host_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/oronbz/visor/internal/hosttest"
	"github.com/oronbz/visor/internal/widget"
)

const secret = "s3cr3t-T0KEN"

// vault is a Widget with one secret-input Action. Its answer can quote the
// secret back, as a careless Widget might.
type vault struct {
	action widget.Action
	ran    *[]widget.Invocation
	answer func(text string) (widget.Result, error)
}

func (v vault) Refresh(context.Context, time.Time) (widget.Payload, error) {
	return widget.Payload{Columns: []widget.Column{{Flex: true}}, Actions: []widget.Action{v.action}}, nil
}

func (v vault) Invoke(_ context.Context, inv widget.Invocation) (widget.Result, error) {
	*v.ran = append(*v.ran, inv)
	if v.answer == nil {
		return widget.Result{}, nil
	}
	return v.answer(inv.Text)
}

func withVault(t *testing.T, a widget.Action, answer func(string) (widget.Result, error)) (*hosttest.Host, *[]widget.Invocation) {
	t.Helper()
	ran := &[]widget.Invocation{}
	typ := widget.Type{Name: "vault", Title: "Vault", New: func(widget.Config) widget.Widget {
		return vault{action: a, ran: ran, answer: answer}
	}}
	h := hosttest.New(t, hosttest.Options{Width: 90, Height: 14, Types: []widget.Type{typ}})
	h.Press("1")
	return h, ran
}

var signIn = widget.Action{ID: "sign-in", Key: "s", Label: "sign in", Input: widget.SecretInput, Prompt: "Paste your token"}

// noSecret fails the test when the secret shows anywhere on screen.
func noSecret(t *testing.T, h *hosttest.Host, when string) {
	t.Helper()
	if strings.Contains(h.Screen(), secret) {
		t.Fatalf("%s the secret is on screen:\n%s", when, h.Screen())
	}
}

func TestASecretPromptMasksWhatIsTypedOrPastedAndHandsItToTheWidget(t *testing.T) {
	h, ran := withVault(t, signIn, nil)
	h.Press("s")
	if _, _, ok := h.Find("Paste your token"); !ok {
		t.Fatalf("no secret prompt:\n%s", h.Screen())
	}
	half := secret[:6]
	for i, r := range half {
		h.Type(string(r))
		noSecret(t, h, "while typing")
		if !strings.Contains(h.Screen(), strings.Repeat("•", i+1)) {
			t.Fatalf("after %d keys the prompt shows no %d dots:\n%s", i+1, i+1, h.Screen())
		}
	}
	if strings.Contains(h.Screen(), half[:3]) {
		t.Fatalf("typed text is echoed:\n%s", h.Screen())
	}
	h.Send(tea.PasteMsg{Content: secret[6:] + "\n"})
	noSecret(t, h, "after pasting")
	if !strings.Contains(h.Screen(), strings.Repeat("•", len(secret))) {
		t.Fatalf("the prompt doesn't count the pasted characters:\n%s", h.Screen())
	}

	h.Press("enter")
	if len(*ran) != 1 || (*ran)[0].Text != secret || (*ran)[0].Action != "sign-in" {
		t.Fatalf("ran %+v, want sign-in with the whole secret", *ran)
	}
	noSecret(t, h, "after enter")
	if _, _, ok := h.Find("Paste your token"); ok {
		t.Errorf("the prompt stays open after enter:\n%s", h.Screen())
	}
}

func TestASecretNeverReachesAToastEvenWhenTheWidgetQuotesIt(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer func(string) (widget.Result, error)
		want   string
	}{
		{"failure", func(s string) (widget.Result, error) { return widget.Result{}, errors.New("bad token " + s) }, "Sign in failed: bad token"},
		{"success", func(s string) (widget.Result, error) { return widget.Result{Toast: "Saved " + s}, nil }, "Saved"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := withVault(t, signIn, tc.answer)
			h.Press("s")
			h.Type(secret)
			h.Press("enter")
			noSecret(t, h, "after the Widget answered")
			if footer := h.Line(13); !strings.Contains(footer, tc.want) {
				t.Errorf("footer %q, want the toast %q", footer, tc.want)
			}
		})
	}
}

func TestASecretPromptStartsEmptyAndEscForgetsIt(t *testing.T) {
	a := signIn
	a.Value = "prefilled"
	h, ran := withVault(t, a, nil)
	h.Press("s")
	if strings.Contains(h.Screen(), "•") || strings.Contains(h.Screen(), "prefilled") {
		t.Fatalf("the secret prompt isn't empty:\n%s", h.Screen())
	}
	h.Type(secret)
	h.Press("esc")
	h.Press("s")
	if strings.Contains(h.Screen(), "•") {
		t.Fatalf("the prompt kept the cancelled secret:\n%s", h.Screen())
	}
	h.Press("esc")
	if len(*ran) != 0 {
		t.Errorf("ran %+v after esc", *ran)
	}
}

func TestAnInputActionWithATargetOpensItAsThePromptAppears(t *testing.T) {
	a := signIn
	a.Target = "https://example.com/tokens/new"
	h, ran := withVault(t, a, nil)
	h.Press("s")
	if got := h.Opener.Opened; len(got) != 1 || got[0] != a.Target {
		t.Fatalf("opened %v, want the token page", got)
	}
	if _, _, ok := h.Find("Paste your token"); !ok {
		t.Fatalf("no secret prompt after opening the page:\n%s", h.Screen())
	}
	h.Type(secret)
	h.Press("enter")
	if len(*ran) != 1 || (*ran)[0].Text != secret {
		t.Fatalf("ran %+v, want the secret handed to the Widget", *ran)
	}
	if len(h.Opener.Opened) != 1 {
		t.Errorf("opened %v, want the page opened once", h.Opener.Opened)
	}
}
