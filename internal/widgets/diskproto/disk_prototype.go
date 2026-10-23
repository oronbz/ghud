// Package diskproto is a PROTOTYPE, throwaway: five disk space Widget variants
// on one tile, cycled with [ and ] while it's focused, to pick the one the
// default layout gets; E, Compact, is the default. f adds fake external
// volumes, r rescans the space hogs.
// Start on a variant with VISOR_DISK_VARIANT=A..E.
package diskproto

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oronbz/visor/internal/render"
	"github.com/oronbz/visor/internal/theme"
	"github.com/oronbz/visor/internal/widget"
	"github.com/shirou/gopsutil/v4/disk"
)

var Type = widget.Type{
	Name:        "disk",
	Title:       "Disk (prototype)",
	Description: "PROTOTYPE: disk space variants.",
	New: func(widget.Config) widget.Widget {
		p := &proto{variant: 4}
		if i := strings.Index("ABCDE", strings.ToUpper(os.Getenv("VISOR_DISK_VARIANT"))); i >= 0 && os.Getenv("VISOR_DISK_VARIANT") != "" {
			p.variant = i
		}
		go p.scan()
		return p
	},
}

var variants = []string{"Hero", "Volumes", "Trend", "Hogs", "Compact"}

const sampleEvery = 5 * time.Second

type vol struct {
	name, mount string
	total, free uint64
}

func (v vol) used() float64 {
	if v.total == 0 {
		return 0
	}
	return float64(v.total-v.free) / float64(v.total)
}

type sample struct {
	at   time.Time
	free uint64
}

type hog struct {
	name, path string
	size       uint64
}

type proto struct {
	mu       sync.Mutex
	variant  int
	fake     bool
	vols     []vol
	sampled  time.Time
	history  []sample
	hogs     []hog
	scanning bool
	scanned  time.Time
}

func (p *proto) Next(now time.Time) time.Time { return now.Truncate(sampleEvery).Add(sampleEvery) }

func (p *proto) Invoke(_ context.Context, inv widget.Invocation) (widget.Result, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	switch inv.Action {
	case "next":
		p.variant = (p.variant + 1) % len(variants)
	case "prev":
		p.variant = (p.variant + len(variants) - 1) % len(variants)
	case "fake":
		p.fake = !p.fake
		return widget.Result{Toast: fmt.Sprintf("fake volumes %v", p.fake)}, nil
	case "rescan":
		if !p.scanning {
			go p.scan()
		}
		return widget.Result{Toast: "rescanning"}, nil
	}
	return widget.Result{Toast: fmt.Sprintf("variant %c · %s", 'A'+p.variant, variants[p.variant])}, nil
}

func (p *proto) Payload(f widget.Frame) widget.Payload {
	p.mu.Lock()
	defer p.mu.Unlock()
	if f.Now.Sub(p.sampled) >= sampleEvery || p.vols == nil {
		p.vols = readVolumes()
		p.sampled = f.Now
		if len(p.vols) > 0 {
			p.history = append(p.history, sample{f.Now, p.vols[0].free})
			if len(p.history) > 720 {
				p.history = p.history[len(p.history)-720:]
			}
		}
	}
	vols := p.vols
	if p.fake {
		vols = append(slices.Clone(vols),
			vol{name: "Backup", mount: "/Volumes/Backup", total: 2_000_000_000_000, free: 412_000_000_000},
			vol{name: "SD Card", mount: "/Volumes/SD Card", total: 128_000_000_000, free: 9_400_000_000})
	}
	var pl widget.Payload
	switch p.variant {
	case 0:
		pl = hero(vols, f.Size)
	case 1:
		pl = volumes(vols)
	case 2:
		pl = p.trend(vols, f.Now)
	case 3:
		pl = p.hogList(vols)
	case 4:
		pl = p.compact(vols, f.Now, f.Size)
	}
	switcher := widget.Line{Spans: []widget.Span{
		{Text: "‹[ ", Token: theme.Faint},
		{Text: fmt.Sprintf("%c · %s", 'A'+p.variant, variants[p.variant]), Token: theme.Alt, Bold: true},
		{Text: " ]›", Token: theme.Faint},
	}}
	if pl.IsList() {
		pl.Lines = append([]widget.Line{switcher}, pl.Lines...)
	} else {
		pl.Lines = append(pl.Lines, widget.Line{}, switcher)
	}
	pl.Actions = append(pl.Actions,
		widget.Action{ID: "prev", Key: "[", Label: "previous variant"},
		widget.Action{ID: "next", Key: "]", Label: "next variant"},
		widget.Action{ID: "fake", Key: "f", Label: "fake volumes"},
		widget.Action{ID: "rescan", Key: "r", Label: "rescan hogs"},
	)
	return pl
}

// A: one big number, the free space on the startup disk.
func hero(vols []vol, size widget.Size) widget.Payload {
	if len(vols) == 0 {
		return widget.Payload{Lines: []widget.Line{widget.Text(theme.Dim, "no volumes")}}
	}
	main := vols[0]
	big := bytesShort(main.free)
	lines := []widget.Line{{Spans: []widget.Span{{Text: big, Token: level(main), Bold: true}}}}
	for _, d := range []widget.Digits{widget.Block, widget.HalfBlock} {
		// Digits, the caption, the bar, the others and the switcher.
		if s := render.DigitsSize(big, d); s.W <= size.W && s.H+5+len(vols)-1 <= size.H {
			lines[0].Digits = d
			break
		}
	}
	lines = append(lines,
		widget.Text(theme.Dim, fmt.Sprintf("free of %s on %s", bytesLong(main.total), main.name)),
		widget.Line{Spans: []widget.Span{
			{Token: level(main), Bar: &widget.Bar{Value: main.used(), Width: min(size.W-6, 30)}},
			{Text: fmt.Sprintf(" %2.0f%%", main.used()*100), Token: theme.Fg},
		}},
	)
	for _, v := range vols[1:] {
		lines = append(lines, widget.Line{Spans: []widget.Span{
			{Text: v.name + " ", Token: theme.Faint},
			{Text: bytesLong(v.free), Token: level(v)},
			{Text: " free", Token: theme.Faint},
		}})
	}
	return widget.Payload{Center: true, Lines: lines}
}

// B: a list, one Item per volume, opening it in Finder.
func volumes(vols []vol) widget.Payload {
	var free uint64
	items := make([]widget.Item, 0, len(vols))
	for _, v := range vols {
		free += v.free
		items = append(items, widget.Item{
			ID: v.mount,
			Fields: []widget.Field{
				widget.F(theme.Fg, v.name),
				{{Token: level(v), Bar: &widget.Bar{Value: v.used()}}},
				{{Text: bytesShort(v.free), Token: level(v), Bold: true}},
				widget.F(theme.Faint, fmt.Sprintf("/ %s", bytesShort(v.total))),
			},
			Actions: []widget.Action{{ID: "open", Label: "open in Finder", Primary: true, Verb: widget.Open, Target: v.mount}},
		})
	}
	return widget.Payload{
		Columns: []widget.Column{{}, {Flex: true, Priority: 1}, {Right: true}, {Priority: 2}},
		Items:   items,
		Count:   len(vols),
		Lines: []widget.Line{{Spans: []widget.Span{
			{Text: bytesLong(free), Token: theme.Bright, Bold: true},
			{Text: " free in all", Token: theme.Dim},
		}}},
	}
}

// E: the Dev box shape, small: half-block free space, one bar line, then the
// free space history in whatever rows are left.
func (p *proto) compact(vols []vol, now time.Time, size widget.Size) widget.Payload {
	if len(vols) == 0 {
		return widget.Payload{Lines: []widget.Line{widget.Text(theme.Dim, "no volumes")}}
	}
	main := vols[0]
	big := bytesShort(main.free) + "B"
	head := widget.Line{Spans: []widget.Span{{Text: big, Token: level(main), Bold: true}}}
	// Digits, the bar line, two chart rows, the caption and the switcher.
	if s := render.DigitsSize(big, widget.HalfBlock); s.W <= size.W && s.H+6 <= size.H {
		head.Digits = widget.HalfBlock
	}
	values, lo, hi := p.chart(main)
	lines := []widget.Line{
		head,
		{Spans: []widget.Span{
			{Text: "free of " + bytesLong(main.total) + " ", Token: theme.Dim},
			{Token: level(main), Bar: &widget.Bar{Value: main.used()}},
			{Text: fmt.Sprintf(" %.0f%% used", main.used()*100), Token: theme.Dim},
		}},
		{Chart: &widget.Chart{Values: values, Token: theme.Accent}},
		widget.Text(theme.Faint, fmt.Sprintf("free, last %s · %s–%s",
			widget.ShortDuration(now.Sub(p.history[0].at).Round(time.Second)), bytesShort(lo), bytesShort(hi))),
	}
	for _, v := range vols[1:] {
		lines = append(lines, widget.Line{Spans: []widget.Span{
			{Text: v.name + " ", Token: theme.Faint},
			{Text: bytesLong(v.free), Token: level(v)},
			{Text: " free", Token: theme.Faint},
		}})
	}
	return widget.Payload{Lines: lines}
}

// chart is the startup disk's free space history, autoscaled with a 1 GB
// floor so a steady disk draws a flat mid line, not noise.
func (p *proto) chart(main vol) (values []float64, lo, hi uint64) {
	lo = main.total
	for _, s := range p.history {
		lo, hi = min(lo, s.free), max(hi, s.free)
	}
	pad := max(int64(1e9)-int64(hi-lo), 0) / 2
	base, span := float64(int64(lo)-pad), float64(int64(hi-lo)+2*pad)
	for _, s := range p.history {
		values = append(values, (float64(s.free)-base)/span)
	}
	return values, lo, hi
}

// C: free space over the session, how fast it's going, and when it runs out.
func (p *proto) trend(vols []vol, now time.Time) widget.Payload {
	if len(vols) == 0 {
		return widget.Payload{Lines: []widget.Line{widget.Text(theme.Dim, "no volumes")}}
	}
	main := vols[0]
	rate := theme.Faint
	rateText := "measuring…"
	full := ""
	if n := len(p.history); n > 1 && p.history[n-1].at.Sub(p.history[0].at) >= time.Minute {
		first, last := p.history[0], p.history[n-1]
		perHour := (float64(last.free) - float64(first.free)) / last.at.Sub(first.at).Hours()
		switch {
		case perHour < -1e8:
			rate, rateText = theme.Warn, fmt.Sprintf("▼ %s/h", bytesShort(uint64(-perHour)))
			full = "full in " + widget.ShortDuration(time.Duration(float64(main.free)/-perHour*float64(time.Hour)))
		case perHour > 1e8:
			rate, rateText = theme.Ok, fmt.Sprintf("▲ %s/h", bytesShort(uint64(perHour)))
		default:
			rateText = "steady"
		}
	}
	values, _, _ := p.chart(main)
	used := []widget.Span{
		{Text: "used ", Token: theme.Dim},
		{Token: level(main), Bar: &widget.Bar{Value: main.used()}},
		{Text: fmt.Sprintf(" %2.0f%%", main.used()*100), Token: theme.Fg},
	}
	if full != "" {
		used = append(used, widget.Span{Text: "  " + full, Token: theme.Warn})
	}
	return widget.Payload{Lines: []widget.Line{
		{Spans: []widget.Span{
			{Text: "free ", Token: theme.Dim},
			{Text: bytesLong(main.free), Token: level(main), Bold: true},
			{Text: "  " + rateText, Token: rate},
		}},
		{Chart: &widget.Chart{Values: values, Token: theme.Accent}},
		{Spans: used},
		widget.Text(theme.Faint, fmt.Sprintf("%s · last %s", main.name, widget.ShortDuration(now.Sub(p.history[0].at).Round(time.Second)))),
	}}
}

// D: what's eating the disk, biggest first, each opening in Finder.
func (p *proto) hogList(vols []vol) widget.Payload {
	summary := []widget.Line{}
	if len(vols) > 0 {
		summary = append(summary, widget.Line{Spans: []widget.Span{
			{Text: bytesLong(vols[0].free), Token: level(vols[0]), Bold: true},
			{Text: " free  ", Token: theme.Dim},
			{Token: level(vols[0]), Bar: &widget.Bar{Value: vols[0].used()}},
		}})
	}
	if p.hogs == nil {
		return widget.Payload{Lines: summary, Columns: []widget.Column{{}}, Empty: "scanning…"}
	}
	biggest := uint64(1)
	for _, h := range p.hogs {
		biggest = max(biggest, h.size)
	}
	items := make([]widget.Item, 0, len(p.hogs))
	var sum uint64
	for _, h := range p.hogs {
		sum += h.size
		items = append(items, widget.Item{
			ID: h.path,
			Fields: []widget.Field{
				widget.F(theme.Fg, h.name),
				{{Token: theme.Info, Bar: &widget.Bar{Value: float64(h.size) / float64(biggest)}}},
				{{Text: bytesShort(h.size), Token: theme.Bright, Bold: true}},
			},
			Actions: []widget.Action{{ID: "open", Label: "open in Finder", Primary: true, Verb: widget.Open, Target: h.path}},
		})
	}
	status := fmt.Sprintf("%s in these · scanned %s ago", bytesShort(sum), widget.ShortDuration(time.Since(p.scanned).Round(time.Second)))
	if p.scanning {
		status = "rescanning…"
	}
	summary = append(summary, widget.Text(theme.Faint, status))
	return widget.Payload{
		Columns: []widget.Column{{}, {Flex: true, Priority: 1}, {Right: true}},
		Count:   len(items),
		Items:   items,
		Lines:   summary,
	}
}

func (p *proto) scan() {
	p.mu.Lock()
	p.scanning = true
	p.mu.Unlock()
	home, _ := os.UserHomeDir()
	candidates := []hog{
		{name: "DerivedData", path: "Library/Developer/Xcode/DerivedData"},
		{name: "Simulators", path: "Library/Developer/CoreSimulator"},
		{name: "Device support", path: "Library/Developer/Xcode/iOS DeviceSupport"},
		{name: "Archives", path: "Library/Developer/Xcode/Archives"},
		{name: "Caches", path: "Library/Caches"},
		{name: "Docker", path: "Library/Containers/com.docker.docker"},
		{name: "Downloads", path: "Downloads"},
		{name: "Trash", path: ".Trash"},
		{name: "Go modules", path: "go/pkg/mod"},
		{name: "npm", path: ".npm"},
		{name: "Gradle", path: ".gradle"},
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var found []hog
	for _, c := range candidates {
		c.path = filepath.Join(home, c.path)
		if _, err := os.Stat(c.path); err != nil {
			continue
		}
		wg.Go(func() {
			// du exits non-zero on unreadable files but still prints the total it could read.
			out, _ := exec.Command("du", "-sk", c.path).Output()
			kb, err := strconv.ParseUint(strings.Fields(string(out) + " 0")[0], 10, 64)
			if err != nil || kb < 100_000 {
				return
			}
			c.size = kb * 1024
			mu.Lock()
			found = append(found, c)
			mu.Unlock()
		})
	}
	wg.Wait()
	slices.SortFunc(found, func(a, b hog) int { return int(int64(b.size)/1e6 - int64(a.size)/1e6) })
	p.mu.Lock()
	p.hogs, p.scanning, p.scanned = found, false, time.Now()
	p.mu.Unlock()
}

// readVolumes is the startup disk, named as /Volumes names it, then each
// other volume mounted under /Volumes, Recovery aside.
func readVolumes() []vol {
	mounted := map[string]bool{}
	if parts, err := disk.Partitions(false); err == nil {
		for _, p := range parts {
			mounted[p.Mountpoint] = true
		}
	}
	entries, _ := os.ReadDir("/Volumes")
	var root vol
	var others []vol
	for _, e := range entries {
		path := filepath.Join("/Volumes", e.Name())
		if target, err := os.Readlink(path); err == nil && target == "/" {
			root = vol{name: e.Name(), mount: "/"}
			continue
		}
		if e.Name() == "Recovery" || !mounted[path] {
			continue
		}
		others = append(others, vol{name: e.Name(), mount: path})
	}
	if root.mount == "" {
		root = vol{name: "Macintosh HD", mount: "/"}
	}
	var out []vol
	for _, v := range append([]vol{root}, others...) {
		u, err := disk.Usage(v.mount)
		if err != nil {
			continue
		}
		v.total, v.free = u.Total, u.Free
		out = append(out, v)
	}
	return out
}

func level(v vol) theme.Token {
	switch {
	case v.used() > 0.9 || v.free < 15e9:
		return theme.Bad
	case v.used() > 0.8:
		return theme.Warn
	}
	return theme.Fg
}

// Decimal units, the way Finder counts.
func bytesShort(b uint64) string {
	switch {
	case b >= 1e12:
		return fmt.Sprintf("%.1fT", float64(b)/1e12)
	case b >= 1e9:
		return fmt.Sprintf("%.0fG", float64(b)/1e9)
	}
	return fmt.Sprintf("%.0fM", float64(b)/1e6)
}

func bytesLong(b uint64) string {
	switch {
	case b >= 1e12:
		return fmt.Sprintf("%.1f TB", float64(b)/1e12)
	case b >= 1e9:
		return fmt.Sprintf("%.0f GB", float64(b)/1e9)
	}
	return fmt.Sprintf("%.0f MB", float64(b)/1e6)
}

// Volume is a mounted volume for the CPU + memory prototype.
type Volume struct {
	Name        string
	Total, Free uint64
	Used        float64
	Level       theme.Token
}

// Volumes is the startup disk, then other volumes; with fake, two made-up
// external ones too.
func Volumes(fake bool) []Volume {
	vols := readVolumes()
	if fake {
		vols = append(vols,
			vol{name: "Backup", total: 2_000_000_000_000, free: 412_000_000_000},
			vol{name: "SD Card", total: 128_000_000_000, free: 9_400_000_000})
	}
	out := make([]Volume, 0, len(vols))
	for _, v := range vols {
		out = append(out, Volume{Name: v.name, Total: v.total, Free: v.free, Used: v.used(), Level: level(v)})
	}
	return out
}

func BytesShort(b uint64) string { return bytesShort(b) }
func BytesLong(b uint64) string  { return bytesLong(b) }
