package render

import (
	"image"
	"image/color"
	"math"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"

	"github.com/oronbz/visor/internal/layout"
	"github.com/oronbz/visor/internal/theme"
	"github.com/oronbz/visor/internal/widget"
)

// Image is a Payload's widget.Image as the host loaded it.
type Image struct {
	// Size is the image's size in pixels, which sets its aspect ratio.
	Size image.Point
	// Cell is a terminal cell's size in pixels.
	Cell image.Point
	// ID, when set, is the kitty graphics image the terminal holds, drawn as
	// Unicode placeholder cells of its virtual Placement, which is exactly
	// the box ImageBox gives.
	ID, Placement uint32
	// Pixels are drawn as truecolor half-blocks when ID isn't set. The host
	// may shrink them ahead of time to the size HalfBlocks gives.
	Pixels image.Image
}

// minLineWidth is the narrowest the Lines beside an Image may get before the
// Image is dropped.
const minLineWidth = 20

// minImageRows is the fewest rows an Image above or below Lines may get
// before it's dropped.
const minImageRows = 3

// imageSplit is where a live view's Image and its Lines go in a body w×h,
// relative to the body; ok is false when the Image is dropped.
func imageSplit(p widget.Payload, pic Image, w, h int) (img, lines layout.Rect, ok bool) {
	if side := p.Image.Side; side == widget.Top || side == widget.Bottom {
		used, fills := linesSpace(p.Lines)
		if p.Wrap {
			used, fills = len(wrapped(theme.Dark(), p, w)), 0
		}
		rows := max(0, h-used) / (fills + 1)
		if rows < minImageRows {
			return layout.Rect{}, layout.Rect{}, false
		}
		cols := min(w, colsFor(pic, rows))
		if cols <= 0 {
			return layout.Rect{}, layout.Rect{}, false
		}
		img, lines = layout.Rect{X: (w - cols) / 2, W: cols, H: rows}, layout.Rect{Y: rows, W: w, H: h - rows}
		if side == widget.Bottom {
			img.Y, lines.Y = h-rows, 0
		}
		return img, lines, true
	}
	cols := colsFor(pic, h)
	lw := w - cols - 1
	if cols <= 0 || lw < minLineWidth {
		return layout.Rect{}, layout.Rect{}, false
	}
	img, lines = layout.Rect{W: cols, H: h}, layout.Rect{X: cols + 1, W: lw, H: h}
	if p.Image.Side == widget.Right {
		img.X, lines.X = lw+1, 0
	}
	return img, lines, true
}

// ImageBox is the size in cells of the box a live view's Image is drawn in,
// in a body w×h; zero when it's left out.
func ImageBox(p widget.Payload, pic Image, w, h int) image.Point {
	if !p.HasImage() {
		return image.Point{}
	}
	at, _, ok := imageSplit(p, pic, w, h)
	if !ok {
		return image.Point{}
	}
	return image.Pt(at.W, at.H)
}

func (pic Image) sized() bool {
	return pic.Size.X > 0 && pic.Size.Y > 0 && pic.Cell.X > 0 && pic.Cell.Y > 0
}

// colsFor is how many cells wide pic is when it's rows cells tall.
func colsFor(pic Image, rows int) int {
	if !pic.sized() {
		return 0
	}
	return max(1, int(math.Round(float64(rows*pic.Cell.Y*pic.Size.X)/float64(pic.Size.Y*pic.Cell.X))))
}

// imageBody is a live view with an Image as exactly h lines of w cells.
func imageBody(th theme.Theme, p widget.Payload, pic Image, l List, w, h int) ([]string, bool) {
	at, rest, ok := imageSplit(p, pic, w, h)
	if !ok {
		return nil, false
	}
	var text []string
	if p.Wrap {
		text = textBody(th, p, l, rest.W, rest.H)
	} else {
		text = liveBody(th, p, rest.W, rest.H)
	}
	cells := placeholders(pic, at.W, at.H)
	if pic.ID == 0 {
		cells = halfBlocks(pic, at.W, at.H)
	}
	out := make([]string, h)
	for y := range out {
		var b strings.Builder
		for x := 0; x < w; {
			switch {
			case x == at.X && y >= at.Y && y < at.Y+at.H:
				b.WriteString(cells[y-at.Y])
				x += at.W
			case x == rest.X && y >= rest.Y && y < rest.Y+rest.H:
				b.WriteString(text[y-rest.Y])
				x += rest.W
			default:
				b.WriteByte(' ')
				x++
			}
		}
		out[y] = b.String()
	}
	return out, true
}

// placeholders are cols×rows kitty Unicode placeholder cells. The 256-colour
// foreground is the ID's low byte and a third diacritic its high one, the
// underline colour the Placement, and the first two diacritics the cell's
// row and column in the image. Indexed colours, unlike RGB ones, come
// through a terminal profile with 256 colours unchanged.
func placeholders(pic Image, cols, rows int) []string {
	st := lipgloss.NewStyle().Foreground(ansi.IndexedColor(pic.ID & 0xff)).UnderlineColor(ansi.IndexedColor(pic.Placement))
	high := ""
	if pic.ID>>24 != 0 {
		high = string(kitty.Diacritic(int(pic.ID >> 24)))
	}
	out := make([]string, rows)
	for r := range out {
		var b strings.Builder
		for c := range cols {
			b.WriteRune(kitty.Placeholder)
			b.WriteRune(kitty.Diacritic(r))
			b.WriteRune(kitty.Diacritic(c))
			b.WriteString(high)
		}
		out[r] = st.Render(b.String())
	}
	return out
}

// HalfBlocks is the size, in pixels of half a cell, that pic is drawn at in
// a box cols×rows cells: fitted inside it, aspect ratio kept.
func HalfBlocks(pic Image, cols, rows int) image.Point {
	if !pic.sized() {
		return image.Point{}
	}
	s := min(float64(cols*pic.Cell.X)/float64(pic.Size.X), float64(rows*pic.Cell.Y)/float64(pic.Size.Y))
	return image.Pt(
		min(cols, max(1, int(math.Round(float64(pic.Size.X)*s/float64(pic.Cell.X))))),
		min(2*rows, max(1, int(math.Round(float64(pic.Size.Y)*s*2/float64(pic.Cell.Y))))),
	)
}

// Shrink scales img to exactly size, each pixel the average of the ones it covers.
func Shrink(img image.Image, size image.Point) image.Image {
	b := img.Bounds()
	if b.Dx() == size.X && b.Dy() == size.Y {
		return img
	}
	out := image.NewRGBA(image.Rectangle{Max: size})
	if b.Empty() {
		return out
	}
	for y := range size.Y {
		y0 := b.Min.Y + y*b.Dy()/size.Y
		y1 := max(y0+1, b.Min.Y+(y+1)*b.Dy()/size.Y)
		for x := range size.X {
			x0 := b.Min.X + x*b.Dx()/size.X
			x1 := max(x0+1, b.Min.X+(x+1)*b.Dx()/size.X)
			var r, g, bl, a, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					cr, cg, cb, ca := img.At(sx, sy).RGBA()
					r, g, bl, a, n = r+uint64(cr), g+uint64(cg), bl+uint64(cb), a+uint64(ca), n+1
				}
			}
			out.Set(x, y, color.RGBA64{R: uint16(r / n), G: uint16(g / n), B: uint16(bl / n), A: uint16(a / n)})
		}
	}
	return out
}

// halfBlocks draws pic centred in cols×rows cells, two pixels a cell: the
// upper one in the foreground of ▀, the lower in its background.
func halfBlocks(pic Image, cols, rows int) []string {
	size := HalfBlocks(pic, cols, rows)
	var px image.Image = image.NewRGBA(image.Rectangle{Max: size})
	if pic.Pixels != nil {
		px = Shrink(pic.Pixels, size)
	}
	b := px.Bounds()
	left, top := (cols-size.X)/2, (2*rows-size.Y)/2
	at := func(x, y int) (color.Color, bool) {
		x, y = x-left, y-top
		if x < 0 || y < 0 || x >= size.X || y >= size.Y {
			return nil, false
		}
		c := px.At(b.Min.X+x, b.Min.Y+y)
		_, _, _, a := c.RGBA()
		return opaque(c), a >= 0x8000
	}
	out := make([]string, rows)
	for r := range out {
		var s strings.Builder
		for c := range cols {
			up, upOk := at(c, 2*r)
			down, downOk := at(c, 2*r+1)
			switch {
			case upOk && downOk:
				s.WriteString(lipgloss.NewStyle().Foreground(up).Background(down).Render("▀"))
			case upOk:
				s.WriteString(lipgloss.NewStyle().Foreground(up).Render("▀"))
			case downOk:
				s.WriteString(lipgloss.NewStyle().Foreground(down).Render("▄"))
			default:
				s.WriteByte(' ')
			}
		}
		out[r] = s.String()
	}
	return out
}

func opaque(c color.Color) color.Color {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	return color.RGBA{R: n.R, G: n.G, B: n.B, A: 255}
}
