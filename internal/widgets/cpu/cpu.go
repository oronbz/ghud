// Package cpu is the CPU + memory Widget type: a braille history of CPU use,
// a memory bar and the load averages.
package cpu

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/oronbz/visor/internal/ports"
	"github.com/oronbz/visor/internal/theme"
	"github.com/oronbz/visor/internal/widget"
)

var Type = widget.Type{
	Name:        "cpu",
	Title:       "CPU + memory",
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
}

func (m *meter) Refresh(context.Context, time.Time) (widget.Payload, error) {
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
	return widget.Payload{Lines: []widget.Line{
		{Spans: []widget.Span{
			{Text: "cpu ", Token: theme.Dim},
			{Text: fmt.Sprintf("%.0f%%", s.CPU*100), Token: level(s.CPU), Bold: true},
		}},
		{Chart: &widget.Chart{Values: samples, Token: theme.Accent}},
		{Spans: []widget.Span{
			{Text: "mem ", Token: theme.Dim},
			{Token: theme.Info, Bar: &widget.Bar{Value: mem}},
			{Text: fmt.Sprintf(" %2.0f%%", mem*100), Token: theme.Fg},
		}},
		widget.Text(theme.Faint, fmt.Sprintf("load %.2f %.2f %.2f", s.Load[0], s.Load[1], s.Load[2])),
	}}, nil
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
