package host_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"

	"github.com/oronbz/visor/internal/hosttest"
	"github.com/oronbz/visor/internal/theme"
	"github.com/oronbz/visor/internal/widget"
)

// pictureView shows whatever Payload its pointer holds when the frame is drawn.
type pictureView struct{ p *widget.Payload }

func (v pictureView) Payload(widget.Frame) widget.Payload { return *v.p }

func pictureType(name string, p *widget.Payload) widget.Type {
	return widget.Type{Name: name, Title: name, New: func(widget.Config) widget.Widget { return pictureView{p} }}
}

// pngOf is a w×h PNG filled with c.
func pngOf(t *testing.T, w, h int, c color.Color) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func withPicture(t *testing.T, w, h int, img *widget.Image, lines ...widget.Line) (*hosttest.Host, *widget.Payload) {
	t.Helper()
	p := &widget.Payload{Lines: lines, Image: img}
	return hosttest.New(t, hosttest.Options{Width: w, Height: h, Types: []widget.Type{pictureType("pic", p)}}), p
}

func dataOf(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

func sameColor(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

var redPx = color.RGBA{R: 255, A: 255}

// kittyTerminal answers the startup queries the way kitty or Ghostty does,
// with 10×20-pixel cells.
func kittyTerminal(h *hosttest.Host) {
	h.Send(uv.KittyGraphicsEvent{Options: kitty.Options{ID: 31}, Payload: []byte("OK")})
	h.Send(uv.CellSizeEvent{Width: 10, Height: 20})
	h.Send(answered)
}

// apc is one kitty graphics escape.
func apc(keys, payload string) string {
	if payload != "" {
		keys += ";" + payload
	}
	return "\x1b_G" + keys + "\x1b\\"
}

// placeholder is the kitty placeholder cell at row r and column c of an image.
func placeholder(r, c int) string {
	return string([]rune{kitty.Placeholder, kitty.Diacritic(r), kitty.Diacritic(c)})
}

func TestWithKittyGraphicsAnImageIsUploadedOnceAndDrawnAsPlaceholders(t *testing.T) {
	data := pngOf(t, 8, 8, redPx)
	h, _ := withPicture(t, 40, 9, &widget.Image{Data: dataOf(data)}, widget.Text(theme.Fg, "hello"))
	kittyTerminal(h)
	h.Press("tab")
	h.Advance(time.Minute)

	upload := apc("a=t,f=100,i=1,q=2", dataOf(data))
	if n := strings.Count(h.Written(), upload); n != 1 {
		t.Errorf("uploaded %d times, want once: %q", n, h.Written())
	}
	// A 38×6 body: a square image in 10×20 cells is 12 cells wide.
	place := apc("a=p,U=1,i=1,p=1,c=12,r=6,q=2", "")
	if n := strings.Count(h.Written(), place); n != 1 || strings.Index(h.Written(), place) < strings.Index(h.Written(), upload) {
		t.Errorf("placed %d times, want once after the upload: %q", n, h.Written())
	}
	for _, at := range []struct{ x, y, r, c int }{{1, 1, 0, 0}, {12, 6, 5, 11}} {
		c := h.Cell(at.x, at.y)
		if c.Content != placeholder(at.r, at.c) || !sameColor(c.Style.Fg, ansi.IndexedColor(1)) || !sameColor(c.Style.UnderlineColor, ansi.IndexedColor(1)) {
			t.Errorf("cell %d,%d: %q fg %v underline %v, want image 1's row %d column %d", at.x, at.y, c.Content, c.Style.Fg, c.Style.UnderlineColor, at.r, at.c)
		}
	}
	if !strings.Contains(h.Line(1), " hello") {
		t.Errorf("no lines beside the image: %q", h.Line(1))
	}
}

func TestALargeImageIsUploadedInChunks(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	noise := rand.New(rand.NewPCG(1, 2))
	for i := range img.Pix {
		img.Pix[i] = byte(noise.IntN(256))
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	h, _ := withPicture(t, 40, 9, &widget.Image{Data: dataOf(b.Bytes())})
	kittyTerminal(h)

	var sent strings.Builder
	var escapes []string
	for _, e := range strings.Split(h.Written(), "\x1b\\") {
		if i := strings.Index(e, "\x1b_G"); i >= 0 {
			escapes = append(escapes, e[i:])
		}
	}
	for i, e := range escapes {
		if !strings.HasPrefix(e, "\x1b_Ga=t,") {
			continue
		}
		if !strings.HasPrefix(e, "\x1b_Ga=t,f=100,i=1,q=2,m=1;") {
			t.Fatalf("first chunk %.40q", e)
		}
		for _, chunk := range escapes[i:] {
			keys, payload, _ := strings.Cut(strings.TrimPrefix(chunk, "\x1b_G"), ";")
			if len(payload) > 4096 {
				t.Errorf("a chunk of %d bytes, over 4096", len(payload))
			}
			sent.WriteString(payload)
			if strings.HasSuffix(keys, "m=0") {
				break
			}
		}
		break
	}
	if sent.String() != dataOf(b.Bytes()) {
		t.Errorf("chunks carry %d bytes, want the %d of the PNG", sent.Len(), len(dataOf(b.Bytes())))
	}
}

func TestWidgetsWithDifferentImagesGetDifferentIDsAndOnesWithTheSameImageShareAnUpload(t *testing.T) {
	red, blue := dataOf(pngOf(t, 8, 8, redPx)), dataOf(pngOf(t, 8, 8, color.RGBA{B: 255, A: 255}))
	a := &widget.Payload{Image: &widget.Image{Data: red, Side: widget.Top}}
	b := &widget.Payload{Image: &widget.Image{Data: blue, Side: widget.Top}}
	c := &widget.Payload{Image: &widget.Image{Data: red, Side: widget.Top}}
	// Three 40×12 tiles: an 18×9 image in each, 10 cells in from the border.
	h := hosttest.New(t, hosttest.Options{Width: 120, Height: 12, Types: []widget.Type{pictureType("a", a), pictureType("b", b), pictureType("c", c)}})
	kittyTerminal(h)

	if n := strings.Count(h.Written(), "a=t,"); n != 2 {
		t.Errorf("%d uploads for two distinct images: %q", n, h.Written())
	}
	for _, want := range []string{apc("a=t,f=100,i=1,q=2", red), apc("a=t,f=100,i=2,q=2", blue)} {
		if !strings.Contains(h.Written(), want) {
			t.Errorf("no upload %.30q…", want)
		}
	}
	ids := map[string]color.Color{}
	for _, id := range []string{"a", "b", "c"} {
		x, y, ok := h.Find(id)
		if !ok {
			t.Fatalf("no tile %s:\n%s", id, h.Screen())
		}
		ids[id] = h.Cell(x+8, y+1).Style.Fg
	}
	if !sameColor(ids["a"], ansi.IndexedColor(1)) || !sameColor(ids["b"], ansi.IndexedColor(2)) || !sameColor(ids["c"], ansi.IndexedColor(1)) {
		t.Errorf("placeholder colours a %v b %v c %v, want 1, 2 and 1", ids["a"], ids["b"], ids["c"])
	}
}

func TestAnImageNoTileShowsIsDeletedAndEveryImageIsDeletedOnQuit(t *testing.T) {
	red, blue := dataOf(pngOf(t, 8, 8, redPx)), dataOf(pngOf(t, 8, 8, color.RGBA{B: 255, A: 255}))
	a := &widget.Payload{Image: &widget.Image{Data: red}}
	b := &widget.Payload{Image: &widget.Image{Data: blue}}
	h := hosttest.New(t, hosttest.Options{Width: 120, Height: 12, Types: []widget.Type{pictureType("a", a), pictureType("b", b)}})
	kittyTerminal(h)

	a.Image = nil
	h.Press("tab")
	if !strings.HasSuffix(h.Written(), apc("a=d,d=I,i=1,q=2", "")) {
		t.Errorf("image 1 isn't deleted once no tile shows it: %q", h.Written())
	}
	a.Image = &widget.Image{Data: red}
	h.Press("tab")
	if !strings.HasSuffix(h.Written(), apc("a=t,f=100,i=1,q=2", red)+apc("a=p,U=1,i=1,p=1,c=18,r=9,q=2", "")) {
		t.Errorf("the image isn't uploaded again, as id 1, once it's back: %q", h.Written())
	}

	h.Press("q")
	if !h.Quit() {
		t.Fatal("q didn't quit")
	}
	if !strings.HasSuffix(h.Written(), apc("a=d,d=I,i=1,q=2", "")+apc("a=d,d=I,i=2,q=2", "")) {
		t.Errorf("quitting leaves images in the terminal: %q", h.Written())
	}
}

func TestAResizedTileGetsANewPlacementAndTheOldOneIsDeleted(t *testing.T) {
	h, _ := withPicture(t, 40, 9, &widget.Image{Data: dataOf(pngOf(t, 8, 8, redPx))}, widget.Text(theme.Fg, "hello"))
	kittyTerminal(h)
	h.Resize(50, 11)

	// A 48×8 body: a 16×8 placement replaces the 12×6 one, the image kept.
	if !strings.HasSuffix(h.Written(), apc("a=p,U=1,i=1,p=2,c=16,r=8,q=2", "")+apc("a=d,d=i,i=1,p=1,q=2", "")) {
		t.Errorf("no refit placement: %q", h.Written())
	}
	if strings.Count(h.Written(), "a=t,") != 1 {
		t.Errorf("the image was uploaded again: %q", h.Written())
	}
	if c := h.Cell(16, 8); c.Content != placeholder(7, 15) || !sameColor(c.Style.UnderlineColor, ansi.IndexedColor(2)) {
		t.Errorf("last image cell %q underline %v, want placement 2's row 7 column 15", c.Content, c.Style.UnderlineColor)
	}
}

func TestTheTerminalsCellSizeSetsAnImagesWidth(t *testing.T) {
	h, _ := withPicture(t, 40, 9, &widget.Image{Data: dataOf(pngOf(t, 8, 8, redPx))}, widget.Text(theme.Fg, "hello"))
	h.Send(uv.KittyGraphicsEvent{Options: kitty.Options{ID: 31}})
	h.Send(uv.CellSizeEvent{Width: 10, Height: 10})
	h.Send(answered)
	if !strings.Contains(h.Written(), apc("a=p,U=1,i=1,p=1,c=6,r=6,q=2", "")) {
		t.Errorf("a square image in square cells isn't 6×6: %q", h.Written())
	}
}

func TestImagesAreReadFromPNGAndJPEGFiles(t *testing.T) {
	dir := t.TempDir()
	pngFile, jpegFile := filepath.Join(dir, "a.png"), filepath.Join(dir, "b.jpg")
	if err := os.WriteFile(pngFile, pngOf(t, 8, 8, redPx), 0o644); err != nil {
		t.Fatal(err)
	}
	var j bytes.Buffer
	if err := jpeg.Encode(&j, image.NewRGBA(image.Rect(0, 0, 16, 8)), nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jpegFile, j.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &widget.Payload{Image: &widget.Image{Path: pngFile, Side: widget.Top}}
	b := &widget.Payload{Image: &widget.Image{Path: jpegFile, Side: widget.Bottom}}
	h := hosttest.New(t, hosttest.Options{Width: 120, Height: 12, Types: []widget.Type{pictureType("a", a), pictureType("b", b)}})
	kittyTerminal(h)

	if !strings.Contains(h.Written(), apc("a=t,f=100,i=1,q=2", dataOf(pngOf(t, 8, 8, redPx)))) {
		t.Errorf("the PNG file isn't uploaded as it is: %q", h.Written())
	}
	_, payload, ok := strings.Cut(h.Written(), "\x1b_Ga=t,f=100,i=2,q=2;")
	payload, _, _ = strings.Cut(payload, "\x1b\\")
	raw, _ := base64.StdEncoding.DecodeString(payload)
	if img, err := png.Decode(bytes.NewReader(raw)); !ok || err != nil || img.Bounds().Size() != image.Pt(16, 8) {
		t.Errorf("the JPEG isn't uploaded as a 16×8 PNG: %v", err)
	}
	// A 58×9 body: the 2:1 JPEG is 36 cells wide at 9 rows.
	if !strings.Contains(h.Written(), apc("a=p,U=1,i=2,p=1,c=36,r=9,q=2", "")) {
		t.Errorf("no 36×9 placement for the JPEG: %q", h.Written())
	}
}

func TestABadImageIsLeftOutAndTheLinesStillDraw(t *testing.T) {
	for name, img := range map[string]widget.Image{
		"missing file": {Path: filepath.Join(t.TempDir(), "gone.png")},
		"not base64":   {Data: "%%%"},
		"not an image": {Data: dataOf([]byte("hello"))},
	} {
		t.Run(name, func(t *testing.T) {
			h, _ := withPicture(t, 40, 9, &img, widget.Text(theme.Fg, "hello"))
			kittyTerminal(h)
			if got := h.Line(1); !strings.HasPrefix(got, "│hello ") {
				t.Errorf("first body row %q, want only the line", got)
			}
			if strings.Contains(h.Written(), "a=t,") {
				t.Errorf("uploaded a bad image: %q", h.Written())
			}
		})
	}
}

func TestAMenuOverATileCoversItsImage(t *testing.T) {
	h, _ := withPicture(t, 40, 9, &widget.Image{Data: dataOf(pngOf(t, 8, 8, redPx))}, widget.Text(theme.Fg, "hello"))
	h.Send(uv.KittyGraphicsEvent{Options: kitty.Options{ID: 31}})
	h.Send(answered)
	h.Press("tab", ".")
	x, y, ok := h.Find("pic")
	if _, my, menu := h.Find("refresh"); !ok || !menu || my <= y {
		t.Fatalf("no menu over the tile:\n%s", h.Screen())
	}
	if c := h.Cell(x+1, y+1); strings.ContainsRune(c.Content, kitty.Placeholder) {
		t.Errorf("the menu's corner at %d,%d is a placeholder cell", x+1, y+1)
	}
}

func TestAnImageItsTileLeavesOutStaysUploadedWhileItsPayloadHasIt(t *testing.T) {
	for _, small := range []struct {
		name string
		w, h int
	}{{"a window too small for the tiles", 4, 2}, {"lines narrower than 20 cells", 30, 9}} {
		t.Run(small.name, func(t *testing.T) {
			h, _ := withPicture(t, 40, 9, &widget.Image{Data: dataOf(pngOf(t, 8, 8, redPx))}, widget.Text(theme.Fg, "hello"))
			kittyTerminal(h)
			h.Resize(small.w, small.h)
			if !strings.HasSuffix(h.Written(), apc("a=d,d=i,i=1,p=1,q=2", "")) {
				t.Errorf("the placement outlives its tile: %q", h.Written())
			}
			if strings.Contains(h.Written(), "a=d,d=I") {
				t.Errorf("the image was deleted while its Payload still has it: %q", h.Written())
			}
			h.Resize(40, 9)
			if !strings.HasSuffix(h.Written(), apc("a=p,U=1,i=1,p=1,c=12,r=6,q=2", "")) {
				t.Errorf("the image isn't placed again: %q", h.Written())
			}
			if n := strings.Count(h.Written(), "a=t,"); n != 1 {
				t.Errorf("uploaded %d times, want once", n)
			}
		})
	}
}

func TestAnImageFileIsReadAgainOnlyWithANewPayload(t *testing.T) {
	file := filepath.Join(t.TempDir(), "a.png")
	blue := pngOf(t, 8, 16, color.RGBA{B: 255, A: 255})
	if err := os.WriteFile(file, pngOf(t, 8, 8, redPx), 0o644); err != nil {
		t.Fatal(err)
	}
	h, p := withPicture(t, 40, 9, &widget.Image{Path: file}, widget.Text(theme.Fg, "hello"))
	kittyTerminal(h)
	if err := os.WriteFile(file, blue, 0o644); err != nil {
		t.Fatal(err)
	}
	h.Send(tea.MouseMotionMsg{X: 5, Y: 3})
	h.Resize(41, 9)
	if n := strings.Count(h.Written(), "a=t,"); n != 1 {
		t.Errorf("uploaded %d times before a new Payload, want once: %q", n, h.Written())
	}
	p.Image = &widget.Image{Path: file}
	h.Press("tab")
	if !strings.Contains(h.Written(), apc("a=t,f=100,i=2,q=2", dataOf(blue))) {
		t.Errorf("a new Payload doesn't read the changed file: %q", h.Written())
	}
}

func TestTheCellSizeIsAskedAgainOnlyWhenAResizedWindowHasImages(t *testing.T) {
	for _, c := range []struct {
		name string
		img  *widget.Image
		want int
	}{{"no image", nil, 1}, {"an image", &widget.Image{Data: dataOf(pngOf(t, 8, 8, redPx))}, 2}} {
		h, _ := withPicture(t, 40, 9, c.img, widget.Text(theme.Fg, "hello"))
		kittyTerminal(h)
		h.Resize(50, 11)
		h.Resize(50, 11)
		if n := strings.Count(h.Written(), "\x1b[16t"); n != c.want {
			t.Errorf("with %s the cell size was asked %d times, want %d", c.name, n, c.want)
		}
	}
}

func TestWithoutImagesNothingButTheQueriesIsWritten(t *testing.T) {
	h, _ := withPicture(t, 40, 9, nil, widget.Text(theme.Fg, "hello"))
	kittyTerminal(h)
	h.Press("tab", "q")
	if strings.Count(h.Written(), "\x1b_G") != 1 {
		t.Errorf("kitty graphics escapes besides the query: %q", h.Written())
	}
}

func TestAWrappedPayloadDrawsItsImageAndScrollsItsLinesBesideIt(t *testing.T) {
	var lines []widget.Line
	for i := range 10 {
		lines = append(lines, widget.Text(theme.Fg, fmt.Sprintf("line %d", i)))
	}
	h, p := withPicture(t, 40, 9, &widget.Image{Data: dataOf(pngOf(t, 8, 8, redPx))}, lines...)
	p.Wrap = true
	h.Advance(time.Second)
	h.Press("tab", "j", "j", "j", "j", "j", "j")

	// A 38×6 body: a 12-cell image, and 10 lines that scroll by 4 at most.
	if got, want := h.Line(1), "│"+strings.Repeat("▀", 12)+"  line 4"; !strings.HasPrefix(got, want) {
		t.Errorf("first body row %q, want %q:\n%s", got, want, h.Screen())
	}
}

// Terminal.app prints an APC's text, and the query goes out before the
// first frame enters the alternate screen, so it would be left on the
// shell's screen at quit.
func TestTheGraphicsQueryIsAskedFromTheAlternateScreen(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 40, Height: 9})
	h.Advance(time.Second)
	want := ansi.SetModeAltScreenSaveCursor + "\x1b_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\" + ansi.ResetModeAltScreenSaveCursor
	if !strings.Contains(h.Written(), want) {
		t.Errorf("wrote %q, want the graphics query inside the alternate screen", h.Written())
	}
}

func TestWithoutAGraphicsAnswerAnImageIsDrawnInHalfBlocks(t *testing.T) {
	h, _ := withPicture(t, 40, 9, &widget.Image{Data: dataOf(pngOf(t, 8, 8, redPx))}, widget.Text(theme.Fg, "hello"))
	h.Advance(time.Second)

	// A 38×6 body: a square image in the assumed 1:2 cells is 12 cells wide.
	if got, want := h.Line(1), "│"+strings.Repeat("▀", 12)+" hello"; !strings.HasPrefix(got, want) {
		t.Errorf("first body row %q, want %q:\n%s", got, want, h.Screen())
	}
	if c := h.Cell(1, 1); !sameColor(c.Style.Fg, redPx) || !sameColor(c.Style.Bg, redPx) {
		t.Errorf("image cell fg %v bg %v, want red on red", c.Style.Fg, c.Style.Bg)
	}
	if strings.Contains(h.Written(), "a=t") {
		t.Errorf("uploaded an image to a terminal without kitty graphics: %q", h.Written())
	}
}
