package host

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"maps"
	"os"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/oronbz/visor/internal/render"
	"github.com/oronbz/visor/internal/widget"
)

// graphicsQuery asks whether the terminal speaks the kitty graphics
// protocol; only one that does answers it, ahead of its device attributes.
const graphicsQuery = "\x1b_Gi=31,s=1,v=1,a=q,t=d,f=24;AAAA\x1b\\"

// cellSizeQueries ask for a cell's size in pixels, and the window's for
// terminals that report only that.
const cellSizeQueries = "\x1b[16t\x1b[14t"

type graphicsMode int

const (
	graphicsAsking graphicsMode = iota
	graphicsHalfBlocks
	graphicsKitty
)

// images are the Payload images the host draws and what the terminal holds
// of them.
type images struct {
	mode graphicsMode
	// answered is whether the graphics query was answered; the mode waits
	// for the device attributes, by when the cell size is in too.
	answered bool
	// cell and window are the sizes the terminal reported, in pixels.
	cell, window image.Point
	loaded       map[string]*loaded
	// shown are the images drawn this frame, by Widget id.
	shown  map[string]render.Picture
	shrunk map[shrinkKey]image.Image
	// uploads are the images the terminal holds, by content hash.
	uploads map[[32]byte]*upload
}

// upload is an image the terminal holds, and its virtual placements by
// their size in cells.
type upload struct {
	n          int
	placements map[image.Point]uint32
}

// id is the kitty image id of the nth upload: 1 to 255 in its low byte, as
// a 256-colour foreground carries it, and what's over in its high byte.
func (u *upload) id() uint32 { return uint32((u.n-1)/255)<<24 | uint32((u.n-1)%255+1) }

type loaded struct {
	hash [32]byte
	img  image.Image
	// png is what's uploaded: the file itself when it's a PNG.
	png []byte
	err error
	// mod and size tell a changed file from the one loaded.
	mod  time.Time
	size int64
}

type shrinkKey struct {
	hash [32]byte
	size image.Point
}

func newImages() *images {
	return &images{loaded: map[string]*loaded{}, shown: map[string]render.Picture{}, shrunk: map[shrinkKey]image.Image{}, uploads: map[[32]byte]*upload{}}
}

// chunk is the most base64 a kitty graphics escape carries.
const chunk = 4096

// upload sends l to the terminal, unless it holds it already.
func (g *images) upload(l *loaded, out *strings.Builder) *upload {
	if u := g.uploads[l.hash]; u != nil {
		return u
	}
	u := &upload{n: 1, placements: map[image.Point]uint32{}}
	for g.taken(u.n) {
		u.n++
	}
	g.uploads[l.hash] = u
	data := base64.StdEncoding.EncodeToString(l.png)
	keys := fmt.Sprintf("a=t,f=100,i=%d,q=2", u.id())
	if len(data) <= chunk {
		out.WriteString(ansi.KittyGraphics([]byte(data), keys))
		return u
	}
	for len(data) > 0 {
		n := min(chunk, len(data))
		more := 0
		if n < len(data) {
			more = 1
		}
		out.WriteString(ansi.KittyGraphics([]byte(data[:n]), keys, fmt.Sprintf("m=%d", more)))
		data, keys = data[n:], "q=2"
	}
	return u
}

func (g *images) taken(n int) bool {
	for _, u := range g.uploads {
		if u.n == n {
			return true
		}
	}
	return false
}

// place gives u a virtual placement the size of box, unless it has one.
func (g *images) place(u *upload, box image.Point, out *strings.Builder) uint32 {
	if p := u.placements[box]; p != 0 {
		return p
	}
	p, taken := uint32(1), slices.Collect(maps.Values(u.placements))
	for slices.Contains(taken, p) {
		p++
	}
	u.placements[box] = p
	out.WriteString(ansi.KittyGraphics(nil, "a=p,U=1", fmt.Sprintf("i=%d,p=%d,c=%d,r=%d,q=2", u.id(), p, box.X, box.Y)))
	return p
}

// prune deletes the placements this frame didn't use, and the images it
// didn't use at all, from the terminal.
func (g *images) prune(used map[*upload]map[image.Point]bool, out *strings.Builder) {
	for _, hash := range g.byID() {
		u := g.uploads[hash]
		if used[u] == nil {
			out.WriteString(ansi.KittyGraphics(nil, fmt.Sprintf("a=d,d=I,i=%d,q=2", u.id())))
			delete(g.uploads, hash)
			continue
		}
		for box, p := range u.placements {
			if !used[u][box] {
				out.WriteString(ansi.KittyGraphics(nil, fmt.Sprintf("a=d,d=i,i=%d,p=%d,q=2", u.id(), p)))
				delete(u.placements, box)
			}
		}
	}
}

// byID is the uploads' hashes in id order, so escapes come out the same
// every time.
func (g *images) byID() [][32]byte {
	hashes := slices.Collect(maps.Keys(g.uploads))
	slices.SortFunc(hashes, func(a, b [32]byte) int { return g.uploads[a].n - g.uploads[b].n })
	return hashes
}

// quit deletes every image visor uploaded from the terminal, then quits.
func (m Model) quit() tea.Cmd {
	var out strings.Builder
	m.images.prune(nil, &out)
	if out.Len() == 0 {
		return tea.Quit
	}
	return tea.Sequence(tea.Raw(out.String()), tea.Quit)
}

// graphicsAnswer reads the terminal's answers to the graphics and cell size
// queries, and asks for the cell size again when the window changes, as a
// font size change does.
func (m Model) graphicsAnswer(msg tea.Msg) tea.Cmd {
	g := m.images
	switch msg := msg.(type) {
	case uv.KittyGraphicsEvent:
		g.answered = g.answered || msg.Options.ID == 31
	case uv.PrimaryDeviceAttributesEvent, colorWaitMsg:
		if g.mode == graphicsAsking {
			g.mode = graphicsHalfBlocks
			if g.answered {
				g.mode = graphicsKitty
			}
		}
	case uv.CellSizeEvent:
		g.cell = image.Pt(msg.Width, msg.Height)
	case uv.PixelSizeEvent:
		g.window = image.Pt(msg.Width, msg.Height)
	case tea.WindowSizeMsg:
		return tea.Raw(cellSizeQueries)
	}
	return nil
}

// cellSize is a cell's size in pixels, or a guess of 1:2 when the terminal
// hasn't said.
func (m Model) cellSize() image.Point {
	g := m.images
	switch {
	case g.cell.X > 0 && g.cell.Y > 0:
		return g.cell
	case g.window.X > 0 && g.window.Y > 0 && m.w > 0 && m.h > 0:
		return image.Pt(g.window.X/m.w, g.window.Y/m.h)
	}
	return image.Pt(1, 2)
}

// syncImages loads the images the frame shows, sizing each to its tile. With
// kitty graphics it uploads and places the ones the terminal doesn't hold
// yet and deletes those no tile shows any more, in escapes written outside
// the frame, ahead of it.
func (m Model) syncImages() tea.Cmd {
	g := m.images
	if g.mode == graphicsAsking || m.w == 0 || m.h == 0 {
		return nil
	}
	var out strings.Builder
	used := map[*upload]map[image.Point]bool{}
	shown := map[string]render.Picture{}
	loaded := map[string]*loaded{}
	shrunk := map[shrinkKey]image.Image{}
	s := m.scene()
	for i, tile := range s.tiles {
		t, p := s.Tiles[i], tile.Payload
		if !p.HasImage() {
			continue
		}
		key, l := g.load(*p.Image)
		loaded[key] = l
		if l.err != nil {
			continue
		}
		pic := render.Picture{Size: l.img.Bounds().Size(), Cell: m.cellSize()}
		box := render.ImageBox(p, pic, t.W-2, t.H-2)
		switch {
		case box == (image.Point{}):
			continue
		case g.mode == graphicsKitty:
			u := g.upload(l, &out)
			pic.ID, pic.Placement = u.id(), g.place(u, box, &out)
			if used[u] == nil {
				used[u] = map[image.Point]bool{}
			}
			used[u][box] = true
		default:
			k := shrinkKey{l.hash, render.HalfBlocks(pic, box.X, box.Y)}
			if shrunk[k] = g.shrunk[k]; shrunk[k] == nil {
				shrunk[k] = render.Shrink(l.img, k.size)
			}
			pic.Pixels = shrunk[k]
		}
		shown[t.ID] = pic
	}
	g.shown, g.loaded, g.shrunk = shown, loaded, shrunk
	g.prune(used, &out)
	if out.Len() == 0 {
		return nil
	}
	return tea.Raw(out.String())
}

// load reads and decodes img, or returns what it read before while the file
// or data is unchanged; key names it among the loaded images.
func (g *images) load(img widget.Image) (string, *loaded) {
	if img.Path == "" {
		key := "data:" + img.Data
		if l := g.loaded[key]; l != nil {
			return key, l
		}
		b, err := base64.StdEncoding.DecodeString(img.Data)
		if err != nil {
			return key, &loaded{err: err}
		}
		return key, decode(b)
	}
	key := "path:" + img.Path
	st, err := os.Stat(img.Path)
	if err != nil {
		return key, &loaded{err: err}
	}
	if l := g.loaded[key]; l != nil && l.mod.Equal(st.ModTime()) && l.size == st.Size() {
		return key, l
	}
	b, err := os.ReadFile(img.Path)
	if err != nil {
		return key, &loaded{err: err}
	}
	l := decode(b)
	l.mod, l.size = st.ModTime(), st.Size()
	return key, l
}

func decode(b []byte) *loaded {
	img, format, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return &loaded{err: err}
	}
	l := &loaded{hash: sha256.Sum256(b), img: img, png: b}
	switch format {
	case "png":
	case "jpeg":
		var out bytes.Buffer
		if err := png.Encode(&out, img); err != nil {
			return &loaded{err: err}
		}
		l.png = out.Bytes()
	default:
		return &loaded{err: errors.New("not a PNG or JPEG")}
	}
	return l
}
