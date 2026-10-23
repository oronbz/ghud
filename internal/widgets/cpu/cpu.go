// Package cpu is the CPU + memory Widget type: a braille history of CPU use,
// a memory bar and the load averages.
package cpu

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/oronbz/visor/internal/ports"
	"github.com/oronbz/visor/internal/theme"
	"github.com/oronbz/visor/internal/widget"
	"github.com/oronbz/visor/internal/widgets/diskproto"
	"github.com/shirou/gopsutil/v4/net"
)

var Type = widget.Type{
	Name:        "cpu",
	Title:       "System",
	Description: "CPU history, memory use and load averages.",
	Refresh:     2 * time.Second,
	Floor:       time.Second,
	New: func(c widget.Config) widget.Widget {
		return &meter{machine: c.Ports.Machine}
	},
}

// history is enough samples for a chart across a wide terminal.
const history = 512

type meter struct {
	machine ports.Machine
	mu      sync.Mutex
	samples []float64
	variant int
	fake    bool
	// rx, tx and at are the previous network read, for the rates.
	rx, tx uint64
	at     time.Time
}

func (m *meter) Refresh(_ context.Context, now time.Time) (widget.Payload, error) {
	s, err := m.machine.Stats()
	if err != nil {
		return widget.Payload{}, err
	}
	m.mu.Lock()
	m.samples = append(m.samples, s.CPU)
	if len(m.samples) > history {
		m.samples = m.samples[len(m.samples)-history:]
	}
	samples := append([]float64(nil), m.samples...)
	m.mu.Unlock()

	var mem float64
	if s.MemTotal > 0 {
		mem = float64(s.MemUsed) / float64(s.MemTotal)
	}
	lines := []widget.Line{
		{Spans: []widget.Span{
			{Text: "cpu ", Token: theme.Dim},
			{Text: fmt.Sprintf("%.0f%%", s.CPU*100), Token: level(s.CPU), Bold: true},
		}},
		{Chart: &widget.Chart{Values: samples, Token: theme.Accent}},
	}
	m.mu.Lock()
	variant, fake := m.variant, m.fake
	m.mu.Unlock()
	disks, width := diskLines(variant, fake, mem, s.MemTotal)
	lines = append(lines, disks...)
	lines = append(lines, m.network(now, width))
	lines = append(lines,
		widget.Line{Spans: []widget.Span{
			{Text: "‹[ ", Token: theme.Faint},
			{Text: fmt.Sprintf("%c · %s", 'A'+variant, diskVariants[variant]), Token: theme.Alt, Bold: true},
			{Text: " ]›", Token: theme.Faint},
		}},
	)
	return widget.Payload{Lines: lines, Actions: []widget.Action{
		{ID: "prev", Key: "[", Label: "previous variant"},
		{ID: "next", Key: "]", Label: "next variant"},
		{ID: "fake", Key: "f", Label: "fake volumes"},
	}}, nil
}

// PROTOTYPE, throwaway: volumes in the CPU + memory Widget, two ways.
var diskVariants = []string{"Bars", "Text"}

func (m *meter) Invoke(_ context.Context, inv widget.Invocation) (widget.Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch inv.Action {
	case "next":
		m.variant = (m.variant + 1) % len(diskVariants)
	case "prev":
		m.variant = (m.variant + len(diskVariants) - 1) % len(diskVariants)
	case "fake":
		m.fake = !m.fake
	}
	return widget.Result{Toast: fmt.Sprintf("%c · %s, fake volumes %v", 'A'+m.variant, diskVariants[m.variant], m.fake)}, nil
}

// diskLines are the memory line and the volumes: A, a bar each with what's
// left beside it; B, the memory bar alone and a line of free space per volume.
func diskLines(variant int, fake bool, mem float64, memTotal uint64) ([]widget.Line, int) {
	vols := diskproto.Volumes(fake)
	name := func(i int) string {
		if i == 0 {
			return "disk"
		}
		return vols[i].Name
	}
	width := len("mem")
	for i := range vols {
		width = max(width, len(name(i)))
	}
	if variant == 1 {
		width = len("mem")
		lines := []widget.Line{{Spans: []widget.Span{
			{Text: "mem ", Token: theme.Dim},
			{Token: theme.Info, Bar: &widget.Bar{Value: mem}},
			{Text: fmt.Sprintf(" %2.0f%%", mem*100), Token: theme.Fg},
		}}}
		for i, v := range vols {
			lines = append(lines, widget.Line{Spans: []widget.Span{
				{Text: name(i) + " ", Token: theme.Dim},
				{Text: diskproto.BytesLong(v.Free), Token: v.Level, Bold: true},
				{Text: " free of " + diskproto.BytesLong(v.Total), Token: theme.Dim},
			}})
		}
		return lines, width
	}
	// Memory in binary units, the way RAM is sold; volumes in decimal, the way Finder counts.
	gib := func(b uint64) string { return fmt.Sprintf("%.0fG", float64(b)/(1<<30)) }
	sizes := []string{gib(uint64((1-mem)*float64(memTotal))) + "/" + gib(memTotal)}
	for _, v := range vols {
		sizes = append(sizes, diskproto.BytesShort(v.Free)+"/"+diskproto.BytesShort(v.Total))
	}
	sizeWidth := 0
	for _, s := range sizes {
		sizeWidth = max(sizeWidth, len(s))
	}
	bar := func(label string, tok theme.Token, used float64, size string) widget.Line {
		return widget.Line{Spans: []widget.Span{
			{Text: fmt.Sprintf("%-*s ", width, label), Token: theme.Dim},
			{Token: tok, Bar: &widget.Bar{Value: used}},
			{Text: fmt.Sprintf(" %3.0f%%", used*100), Token: theme.Fg},
			{Text: fmt.Sprintf(" %*s", sizeWidth, size), Token: theme.Dim},
		}}
	}
	lines := []widget.Line{bar("mem", theme.Info, mem, sizes[0])}
	for i, v := range vols {
		tok := v.Level
		if tok == theme.Fg {
			tok = theme.Alt
		}
		lines = append(lines, bar(name(i), tok, v.Used, sizes[i+1]))
	}
	return lines, width
}

func level(cpu float64) theme.Token {
	switch {
	case cpu > 0.85:
		return theme.Bad
	case cpu > 0.65:
		return theme.Warn
	}
	return theme.Fg
}

// network is the download and upload rate since the previous Refresh, summed
// over the en* interfaces: Wi-Fi and Ethernet, but not loopback, AirDrop or
// a VPN's utun, which would count the same traffic twice.
func (m *meter) network(now time.Time, width int) widget.Line {
	var rx, tx uint64
	if nics, err := net.IOCounters(true); err == nil {
		for _, n := range nics {
			if strings.HasPrefix(n.Name, "en") {
				rx, tx = rx+n.BytesRecv, tx+n.BytesSent
			}
		}
	}
	m.mu.Lock()
	prx, ptx, pat := m.rx, m.tx, m.at
	m.rx, m.tx, m.at = rx, tx, now
	m.mu.Unlock()
	down, up := "–", "–"
	if secs := now.Sub(pat).Seconds(); !pat.IsZero() && secs > 0 && rx >= prx && tx >= ptx {
		down, up = rate(float64(rx-prx)/secs), rate(float64(tx-ptx)/secs)
	}
	return widget.Line{Spans: []widget.Span{
		{Text: fmt.Sprintf("%-*s ", width, "net"), Token: theme.Dim},
		{Text: "↓ ", Token: theme.Ok},
		{Text: down, Token: theme.Fg},
		{Text: "  ↑ ", Token: theme.Info},
		{Text: up, Token: theme.Fg},
	}}
}

// rate is bytes per second in decimal units, the way Activity Monitor shows them.
func rate(b float64) string {
	switch {
	case b >= 1e9:
		return fmt.Sprintf("%.1f GB/s", b/1e9)
	case b >= 1e6:
		return fmt.Sprintf("%.1f MB/s", b/1e6)
	case b >= 1e3:
		return fmt.Sprintf("%.0f KB/s", b/1e3)
	}
	return fmt.Sprintf("%.0f B/s", b)
}
