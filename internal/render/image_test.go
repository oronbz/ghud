package render_test

import (
	"image"
	"image/color"
	"strings"
	"testing"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"

	"github.com/oronbz/visor/internal/render"
	"github.com/oronbz/visor/internal/theme"
	"github.com/oronbz/visor/internal/widget"
)

var red = color.RGBA{R: 255, A: 255}

// square is a w×h image filled with c.
func square(w, h int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	return img
}

// tall is a cell twice as high as it's wide, as most terminal fonts draw.
var tall = image.Pt(10, 20)

func pictured(side widget.Side, lines ...widget.Line) widget.Payload {
	return widget.Payload{Lines: lines, Image: &widget.Image{Data: "x", Side: side}}
}

// inner is a tile's body as plain text, one string per row, border cut off.
func inner(t render.Tile) []string {
	rows := strings.Split(ansi.Strip(render.Draw(theme.Dark(), t)), "\n")
	out := make([]string, 0, len(rows)-2)
	for _, r := range rows[1 : len(rows)-1] {
		rs := []rune(r)
		out = append(out, string(rs[1:len(rs)-1]))
	}
	return out
}

func TestAnImageOnTheLeftTakesTheBodyHeightAndTheLinesTheWidthLeftAfterAGap(t *testing.T) {
	// A square image in 1:2 cells is twice as many cells wide as it is rows tall.
	for _, size := range []struct{ w, h, cols int }{{40, 8, 12}, {60, 10, 16}} {
		pic := render.Picture{Size: image.Pt(40, 40), Cell: tall, Pixels: square(40, 40, red)}
		body := inner(render.Tile{W: size.w, H: size.h, Payload: pictured(widget.Left, widget.Text(theme.Fg, "hello")), Picture: &pic})
		for i, row := range body {
			want := strings.Repeat("▀", size.cols)
			if i == 0 {
				want += " hello"
			}
			if got := strings.TrimRight(row, " "); got != want {
				t.Errorf("%d×%d row %d: %q, want %q", size.w, size.h, i, got, want)
			}
		}
	}
}

func TestHalfBlockCellsCarryTheImageColours(t *testing.T) {
	pic := render.Picture{Size: image.Pt(2, 2), Cell: tall, Pixels: square(2, 2, red)}
	out := render.Draw(theme.Dark(), render.Tile{W: 30, H: 3, Payload: pictured(widget.Left), Picture: &pic})
	buf := uv.NewScreenBuffer(30, 3)
	uv.NewStyledString(out).Draw(buf, buf.Bounds())
	c := buf.CellAt(1, 1)
	if c.Content != "▀" || !same(c.Style.Fg, red) || !same(c.Style.Bg, red) {
		t.Errorf("image cell %q fg %v bg %v, want ▀ red on red", c.Content, c.Style.Fg, c.Style.Bg)
	}
}

func TestAnImageOnTheRightLeavesTheLinesOnTheLeftAndCentreCentresThemInTheirArea(t *testing.T) {
	pic := render.Picture{Size: image.Pt(40, 40), Cell: tall, Pixels: square(40, 40, red)}
	p := pictured(widget.Right, widget.Text(theme.Fg, "hi"))
	p.Center = true
	// 40×7: a 38×5 body, a 10-cell image, and 27 cells of lines before the gap.
	body := inner(render.Tile{W: 40, H: 7, Payload: p, Picture: &pic})
	for i, row := range body {
		lines, img := string([]rune(row)[:27]), string([]rune(row)[28:])
		want := strings.Repeat(" ", 27)
		if i == 2 {
			want = strings.Repeat(" ", 12) + "hi" + strings.Repeat(" ", 13)
		}
		if lines != want || img != strings.Repeat("▀", 10) {
			t.Errorf("row %d: %q | %q, want %q | the image", i, lines, img, want)
		}
	}
}

func TestAnImageBesideLinesNarrowerThanTwentyCellsIsDropped(t *testing.T) {
	pic := render.Picture{Size: image.Pt(40, 40), Cell: tall, Pixels: square(40, 40, red)}
	p := pictured(widget.Left, widget.Text(theme.Fg, "hello"))
	// 34×7: a 32×5 body; the 10-cell image and its gap leave 21 cells, and 19 at 32×7.
	if body := inner(render.Tile{W: 34, H: 7, Payload: p, Picture: &pic}); !strings.HasPrefix(body[0], "▀▀▀▀▀▀▀▀▀▀ hello") {
		t.Errorf("at 21 cells of lines the image is gone: %q", body[0])
	}
	got := inner(render.Tile{W: 32, H: 7, Payload: p, Picture: &pic})
	want := inner(render.Tile{W: 32, H: 7, Payload: widget.Payload{Lines: p.Lines}})
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("at 19 cells of lines:\n%s\nwant only the lines:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestAnImageOnTopTakesTheRowsTheLinesLeaveCentredAcross(t *testing.T) {
	pic := render.Picture{Size: image.Pt(40, 40), Cell: tall, Pixels: square(40, 40, red)}
	// 30×12: a 28×10 body; one line leaves 9 rows, an 18-cell-wide square.
	body := inner(render.Tile{W: 30, H: 12, Payload: pictured(widget.Top, widget.Text(theme.Fg, "hello")), Picture: &pic})
	img := strings.Repeat(" ", 5) + strings.Repeat("▀", 18) + strings.Repeat(" ", 5)
	for i, row := range body[:9] {
		if row != img {
			t.Errorf("row %d: %q, want the image centred", i, row)
		}
	}
	if got := strings.TrimRight(body[9], " "); got != "hello" {
		t.Errorf("last row %q, want the line under the image", got)
	}
}

func TestAnImageAtTheBottomSharesTheRowsLeftWithAFillChart(t *testing.T) {
	pic := render.Picture{Size: image.Pt(40, 40), Cell: tall, Pixels: square(40, 40, red)}
	fill := widget.Line{Chart: &widget.Chart{Values: []float64{1, 1}, Token: theme.Ok}}
	// 30×12: a 28×10 body; the line leaves 9 rows, 4 for the image and 5 for the chart.
	body := inner(render.Tile{W: 30, H: 12, Payload: pictured(widget.Bottom, widget.Text(theme.Fg, "hello"), fill), Picture: &pic})
	for i := 1; i <= 5; i++ {
		if !strings.Contains(body[i], "⣿") {
			t.Errorf("row %d %q isn't chart", i, body[i])
		}
	}
	img := strings.Repeat(" ", 10) + strings.Repeat("▀", 8) + strings.Repeat(" ", 10)
	for i, row := range body[6:] {
		if row != img {
			t.Errorf("image row %d: %q, want %q", i, row, img)
		}
	}
}

func TestAWideImageAboveLinesIsCappedAtTheBodyWidth(t *testing.T) {
	pic := render.Picture{Size: image.Pt(200, 40), Cell: tall, Pixels: square(200, 40, red)}
	body := inner(render.Tile{W: 30, H: 12, Payload: pictured(widget.Top, widget.Text(theme.Fg, "hello")), Picture: &pic})
	if !slicesContain(body, strings.Repeat("▀", 28)) {
		t.Errorf("no image row across the whole body:\n%s", strings.Join(body, "\n"))
	}
}

func TestAnImageUnderThreeRowsAboveOrBelowLinesIsDropped(t *testing.T) {
	pic := render.Picture{Size: image.Pt(40, 40), Cell: tall, Pixels: square(40, 40, red)}
	for _, side := range []widget.Side{widget.Top, widget.Bottom} {
		p := pictured(side, widget.Text(theme.Fg, "one"), widget.Text(theme.Fg, "two"))
		// 30×6: a 4-row body; two lines leave 2.
		got := inner(render.Tile{W: 30, H: 6, Payload: p, Picture: &pic})
		want := inner(render.Tile{W: 30, H: 6, Payload: widget.Payload{Lines: p.Lines}})
		if strings.Join(got, "\n") != strings.Join(want, "\n") {
			t.Errorf("side %d:\n%s\nwant only the lines", side, strings.Join(got, "\n"))
		}
		if got := inner(render.Tile{W: 30, H: 7, Payload: p, Picture: &pic}); !slicesContain(got, strings.Repeat(" ", 11)+"▀▀▀▀▀▀"+strings.Repeat(" ", 11)) {
			t.Errorf("side %d at 3 rows, no image:\n%s", side, strings.Join(got, "\n"))
		}
	}
}

func TestAnUploadedImageIsDrawnAsPlaceholderCellsEncodingItsIDPlacementAndPosition(t *testing.T) {
	for _, id := range []uint32{7, 3<<24 | 5} {
		pic := render.Picture{Size: image.Pt(40, 40), Cell: tall, ID: id, Placement: 2}
		// 40×5: a 3-row body and a 6-cell image.
		out := render.Draw(theme.Dark(), render.Tile{W: 40, H: 5, Payload: pictured(widget.Left, widget.Text(theme.Fg, "hi")), Picture: &pic})
		buf := uv.NewScreenBuffer(40, 5)
		uv.NewStyledString(out).Draw(buf, buf.Bounds())
		for r := range 3 {
			for c := range 6 {
				cell := buf.CellAt(1+c, 1+r)
				want := string([]rune{kitty.Placeholder, kitty.Diacritic(r), kitty.Diacritic(c)})
				if id>>24 != 0 {
					want += string(kitty.Diacritic(int(id >> 24)))
				}
				if cell.Content != want || cell.Width != 1 {
					t.Fatalf("id %d cell %d,%d: %q (width %d), want %q", id, c, r, cell.Content, cell.Width, want)
				}
				if !same(cell.Style.Fg, ansi.IndexedColor(id&0xff)) || !same(cell.Style.UnderlineColor, ansi.IndexedColor(2)) {
					t.Fatalf("id %d cell %d,%d: fg %v underline %v, want colour %d over %d", id, c, r, cell.Style.Fg, cell.Style.UnderlineColor, id&0xff, 2)
				}
				if cell.Style.Underline != uv.UnderlineNone {
					t.Fatalf("cell %d,%d is underlined", c, r)
				}
			}
		}
		if got := buf.CellAt(8, 1).Content; got != "h" {
			t.Errorf("id %d: lines start with %q, want h after the gap", id, got)
		}
	}
}

func slicesContain(rows []string, s string) bool {
	for _, r := range rows {
		if strings.Contains(r, s) {
			return true
		}
	}
	return false
}

func same(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}
