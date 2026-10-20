package render_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/oronbz/visor/internal/render"
	"github.com/oronbz/visor/internal/theme"
	"github.com/oronbz/visor/internal/widget"
)

// drawDigits is a digits line's rows as a tile draws them, without the
// border and with trailing spaces trimmed.
func drawDigits(text string, d widget.Digits) []string {
	p := widget.Payload{Lines: []widget.Line{{Digits: d, Spans: []widget.Span{{Text: text, Token: theme.Fg}}}}}
	rows := render.DigitsSize("", d).H
	tile := strings.Split(ansi.Strip(render.Draw(theme.Dark(), render.Tile{W: 80, H: rows + 2, Payload: p})), "\n")
	out := make([]string, rows)
	for i := range out {
		r := []rune(tile[i+1])
		out[i] = strings.TrimRight(string(r[1:len(r)-1]), " ")
	}
	return out
}

func width(rows []string) int {
	w := 0
	for _, r := range rows {
		w = max(w, len([]rune(r)))
	}
	return w
}

func TestCountersDrawEveryCharacter(t *testing.T) {
	// Half-block widths: 3 a digit or letter, 5 for M, 1 for . and ,, one gap between glyphs.
	for text, half := range map[string]int{
		"12.4M":  3 + 3 + 1 + 3 + 5 + 4,
		"312GB":  5*3 + 4,
		"1,024K": 3 + 1 + 4*3 + 5,
		"-3%":    3*3 + 2,
		"2TB":    3*3 + 2,
	} {
		for d, want := range map[widget.Digits]int{widget.HalfBlock: half, widget.Block: 2 * half} {
			if got := render.DigitsSize(text, d).W; got != want {
				t.Errorf("%q at %v measures %d wide, want %d", text, d, got, want)
			}
			if got := width(drawDigits(text, d)); got != want {
				t.Errorf("%q at %v draws %d wide, want %d:\n%s", text, d, got, want, strings.Join(drawDigits(text, d), "\n"))
			}
		}
	}
}

func TestEachNewGlyphMeasuresWhatItDraws(t *testing.T) {
	for text, dots := range map[string]int{".": 1, ",": 1, "%": 3, "K": 3, "G": 3, "B": 3, "T": 3, "-": 3} {
		for d, scale := range map[widget.Digits]int{widget.HalfBlock: 1, widget.Block: 2} {
			rows := drawDigits(text, d)
			size := render.DigitsSize(text, d)
			if size.W != dots*scale || width(rows) != size.W || len(rows) != size.H {
				t.Errorf("%q at %v measures %v, draws %d wide and %d tall, want %d wide:\n%s", text, d, size, width(rows), len(rows), dots*scale, strings.Join(rows, "\n"))
			}
		}
	}
}

func TestPeriodAndCommaDiffer(t *testing.T) {
	for _, d := range []widget.Digits{widget.HalfBlock, widget.Block} {
		period, comma := strings.Join(drawDigits("1.2", d), "\n"), strings.Join(drawDigits("1,2", d), "\n")
		if period == comma {
			t.Errorf("1.2 and 1,2 draw the same at %v:\n%s", d, period)
		}
	}
}

func TestNewGlyphsAtHalfBlock(t *testing.T) {
	want := []string{
		"        ▀ █ █▄▀ █▀▀ █▀▄ ▀█▀",
		"  ▄ ▀▀▀ ▄▀  █▄  █ █ █▀▄  █",
		"▀ ▀     ▀ ▀ ▀ ▀ ▀▀▀ ▀▀   ▀",
	}
	if got := drawDigits(".,-%KGBT", widget.HalfBlock); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestLowercaseDrawsAsUppercase(t *testing.T) {
	for lower, upper := range map[string]string{"312gb": "312GB", "3pm": "3PM", "12.4k": "12.4K", "2tb": "2TB"} {
		for _, d := range []widget.Digits{widget.HalfBlock, widget.Block} {
			got, want := strings.Join(drawDigits(lower, d), "\n"), strings.Join(drawDigits(upper, d), "\n")
			if got != want {
				t.Errorf("%q at %v:\n%s\nwant as %q:\n%s", lower, d, got, upper, want)
			}
			if got, want := render.DigitsSize(lower, d), render.DigitsSize(upper, d); got != want {
				t.Errorf("%q at %v measures %v, %q %v", lower, d, got, upper, want)
			}
		}
	}
}

func TestClockDigitsAreUnchanged(t *testing.T) {
	for _, tc := range []struct {
		text string
		d    widget.Digits
		want []string
	}{
		{"12:30", widget.HalfBlock, []string{
			" ▀█ ▀▀█ ▄ ▀▀█ █▀█",
			"  █ █▀▀ ▄  ▀█ █ █",
			"  ▀ ▀▀▀   ▀▀▀ ▀▀▀",
		}},
		{"12:30", widget.Block, []string{
			"  ████  ██████      ██████  ██████",
			"    ██      ██  ██      ██  ██  ██",
			"    ██  ██████        ████  ██  ██",
			"    ██  ██      ██      ██  ██  ██",
			"    ██  ██████      ██████  ██████",
		}},
		{"9:05 PM", widget.HalfBlock, []string{
			"█▀█ ▄ █▀█ █▀▀   █▀▄ █▄ ▄█",
			"▀▀█ ▄ █ █ ▀▀█   █▀  █ ▀ █",
			"▀▀▀   ▀▀▀ ▀▀▀   ▀   ▀   ▀",
		}},
		{"9:05 PM", widget.Block, []string{
			"██████      ██████  ██████      ████    ██      ██",
			"██  ██  ██  ██  ██  ██          ██  ██  ████  ████",
			"██████      ██  ██  ██████      ████    ██  ██  ██",
			"    ██  ██  ██  ██      ██      ██      ██      ██",
			"██████      ██████  ██████      ██      ██      ██",
		}},
	} {
		got := strings.Join(drawDigits(tc.text, tc.d), "\n")
		if want := strings.Join(tc.want, "\n"); got != want {
			t.Errorf("%q at %v:\n%s\nwant:\n%s", tc.text, tc.d, got, want)
		}
	}
}
