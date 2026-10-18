package host_test

import (
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/theme"
)

func TestFooterShowsTheNormalBadgeAndGlobalHintsBelowTheGrid(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 100, Height: 21})

	if got := h.Line(19); !strings.HasPrefix(got, "╰") {
		t.Errorf("the grid's last line is %q, want bottom borders right above the footer", got)
	}
	footer := h.Line(20)
	if !strings.HasPrefix(footer, " NORMAL ") {
		t.Fatalf("footer %q does not start with the NORMAL badge", footer)
	}
	for x := range len(" NORMAL ") {
		if bg, ok := h.Bg(x, 20); !ok || bg != theme.Accent {
			t.Errorf("badge cell %d background %v (theme colour %v), want accent", x, bg, ok)
		}
	}
	if fg, ok := h.Fg(1, 20); !ok || fg != theme.Bg || !h.Bold(1, 20) {
		t.Errorf("badge text drawn in %v (theme colour %v, bold %v), want bold bg", fg, ok, h.Bold(1, 20))
	}
	for _, hint := range []string{"Tab", "1–9", "? keys", "q quit"} {
		if !strings.Contains(footer, hint) {
			t.Errorf("unfocused footer %q lacks the hint %q", footer, hint)
		}
	}

	h.Press("tab")
	if footer := h.Line(20); !strings.Contains(footer, "esc unfocus") {
		t.Errorf("focused footer %q lacks the esc hint", footer)
	}
}
