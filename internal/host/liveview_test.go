package host_test

import (
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/widget"
	"github.com/oronbz/ghud/internal/widgets/clock"
)

func TestLargeClockDrawsBlockDigits(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Types: []widget.Type{clock.Type}, Width: 80, Height: 13, Now: sundayAfternoon})

	// "14" in 3×5 block glyphs, each dot two cells wide: rows 0, 2 and 4.
	x, y, ok := h.Find("  ████  ██  ██")
	if !ok {
		t.Fatalf("no block digits:\n%s", h.Screen())
	}
	if got := cells(h.Line(y+2), x, 14); got != "    ██  ██████" {
		t.Errorf("middle row %q, want %q", got, "    ██  ██████")
	}
	if got := cells(h.Line(y+4), x, 14); got != "    ██      ██" {
		t.Errorf("bottom row %q, want %q", got, "    ██      ██")
	}
	if strings.ContainsAny(h.Screen(), "▀▄") {
		t.Errorf("half blocks in a large clock:\n%s", h.Screen())
	}
	if _, dy, ok := h.Find("Sunday 18 October"); !ok || dy <= y+4 {
		t.Errorf("date not below the digits:\n%s", h.Screen())
	}
}

func TestTwelveHourClockShowsPM(t *testing.T) {
	cfg := `time_format = "12h"`
	small := hosttest.New(t, hosttest.Options{Types: []widget.Type{clock.Type}, Config: cfg, Width: 30, Height: 6, Now: sundayAfternoon})
	if _, _, ok := small.Find("2:05:09 PM"); !ok {
		t.Errorf("small 12h clock:\n%s", small.Screen())
	}

	big := hosttest.New(t, hosttest.Options{Types: []widget.Type{clock.Type}, Config: cfg, Width: 50, Height: 9, Now: sundayAfternoon})
	// P then M in half-block glyphs, top row.
	if _, _, ok := big.Find("█▀▄ █▄ ▄█"); !ok {
		t.Errorf("half-block 12h clock has no PM:\n%s", big.Screen())
	}
}

// cells is n cells of line from x, padded with spaces.
func cells(line string, x, n int) string {
	r := []rune(line + strings.Repeat(" ", x+n))
	return string(r[x : x+n])
}
