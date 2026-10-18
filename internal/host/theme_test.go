package host_test

import (
	"fmt"
	"image/color"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/oronbz/ghud/internal/hosttest"
)

func hexOf(c color.Color) string {
	if c == nil {
		return "none"
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

// fgHex is the foreground of the first cell showing text.
func fgHex(t *testing.T, h *hosttest.Host, text string) string {
	t.Helper()
	x, y, ok := h.Find(text)
	if !ok {
		t.Fatalf("%q not on screen:\n%s", text, h.Screen())
	}
	return hexOf(h.Cell(x, y).Style.Fg)
}

// answered is the terminal's reply to the primary device attributes query,
// which follows every colour answer it was going to give.
var answered = uv.PrimaryDeviceAttributesEvent{62, 22}

func TestALightTerminalBackgroundSelectsTheLightPalette(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 40, Height: 9})

	h.Send(tea.BackgroundColorMsg{Color: lipgloss.Color("#fdf6e3")})
	h.Send(answered)

	if got := fgHex(t, h, "╭"); got != "#c5ccd8" {
		t.Errorf("border %s, want the light palette's line #c5ccd8", got)
	}
	if got := fgHex(t, h, "Sun 18"); got != "#6b7383" {
		t.Errorf("date %s, want the light palette's dim #6b7383", got)
	}
	if got := hexOf(h.View().BackgroundColor); got != "#f7f8fa" {
		t.Errorf("terminal background set to %s, want the light palette's bg #f7f8fa", got)
	}
}

func TestADarkTerminalBackgroundKeepsTheDarkPalette(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 40, Height: 9})

	h.Send(tea.BackgroundColorMsg{Color: lipgloss.Color("#1e1e2e")})
	h.Send(answered)

	if got := fgHex(t, h, "╭"); got != "#3d4757" {
		t.Errorf("border %s, want the dark palette's line #3d4757", got)
	}
	if got := hexOf(h.View().BackgroundColor); got != "#13171d" {
		t.Errorf("terminal background set to %s, want the dark palette's bg #13171d", got)
	}
}

// osc4 is a terminal's answer to an OSC 4 palette query.
func osc4(index int, spec string) uv.UnknownOscEvent {
	return uv.UnknownOscEvent(fmt.Sprintf("\x1b]4;%d;%s\x1b\\", index, spec))
}

func TestTerminalThemeDerivesTokensFromTheTerminalsColours(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 40, Height: 9, Config: `theme = "terminal"`})

	h.Send(tea.BackgroundColorMsg{Color: lipgloss.Color("#000000")})
	h.Send(tea.ForegroundColorMsg{Color: lipgloss.Color("#ffffff")})
	h.Send(osc4(1, "rgb:cccc/0000/0000"))
	h.Send(osc4(4, "rgb:1111/2222/eeee"))
	h.Send(answered)

	if got := fgHex(t, h, "Clock"); got != "#ffffff" {
		t.Errorf("title %s, want the terminal foreground #ffffff", got)
	}
	if got := fgHex(t, h, "╭"); got != "#383838" {
		t.Errorf("border %s, want line blended 22%% from fg over bg, #383838", got)
	}
	if got := fgHex(t, h, "Sun 18"); got != "#8c8c8c" {
		t.Errorf("date %s, want dim blended 55%% from fg over bg, #8c8c8c", got)
	}
	if got := hexOf(h.View().BackgroundColor); got != "none" {
		t.Errorf("terminal background set to %s, want the terminal's own left alone", got)
	}

	h.Press("tab")
	if got := fgHex(t, h, "╭"); got != "#1122ee" {
		t.Errorf("focused border %s, want accent from palette colour 4 (blue), #1122ee", got)
	}
}

func TestTerminalThemeFallsBackToGhudWithoutAnswers(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 40, Height: 9, Config: `theme = "terminal"`})

	h.Send(answered)

	if got := fgHex(t, h, "╭"); got != "#3d4757" {
		t.Errorf("border %s, want the ghud dark palette's line #3d4757", got)
	}
	if got := hexOf(h.View().BackgroundColor); got != "#13171d" {
		t.Errorf("terminal background %s, want the ghud dark palette's bg #13171d", got)
	}
}

func TestWithNoAnswerTheDarkPaletteIsUsed(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 40, Height: 9})

	if got := hexOf(h.View().BackgroundColor); got != "none" {
		t.Errorf("terminal background set to %s before the terminal could answer the query", got)
	}
	h.Advance(time.Second)

	if got := fgHex(t, h, "╭"); got != "#3d4757" {
		t.Errorf("border %s, want the dark palette's line #3d4757", got)
	}
	if got := hexOf(h.View().BackgroundColor); got != "#13171d" {
		t.Errorf("terminal background set to %s after the query timed out, want the dark palette's bg #13171d", got)
	}
}
