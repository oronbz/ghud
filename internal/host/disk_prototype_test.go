package host_test

// PROTOTYPE, throwaway: prints the CPU + memory Widget with volumes, each
// variant, in a lone tile and in the default layout.
// go test ./internal/host -run DiskPrototype -v

import (
	"fmt"
	"testing"
	"time"

	"github.com/oronbz/visor/internal/host"
	"github.com/oronbz/visor/internal/hosttest"
	"github.com/oronbz/visor/internal/ports"
	"github.com/oronbz/visor/internal/widget"
	"github.com/oronbz/visor/internal/widgets/cpu"
)

func TestDiskPrototype(t *testing.T) {
	stats := ports.Stats{CPU: 0.6, MemUsed: 12 << 30, MemTotal: 16 << 30, Load: [3]float64{2.14, 1.96, 1.71}}
	for _, size := range [][2]int{{38, 14}, {30, 10}} {
		h := hosttest.New(t, hosttest.Options{Types: []widget.Type{cpu.Type}, Width: size[0], Height: size[1]})
		h.Machine.Set(stats)
		for i := 0; i < 30; i++ {
			stats.CPU = float64(i%7) / 7
			h.Machine.Set(stats)
			h.Advance(2 * time.Second)
		}
		h.Press("tab")
		for _, fake := range []bool{false, true} {
			for v := 0; v < 2; v++ {
				h.Advance(2 * time.Second)
				fmt.Printf("=== %dx%d fake=%v variant %d\n%s\n", size[0], size[1], fake, v, h.Screen())
				h.Press("]")
			}
			h.Press("f")
		}
	}
	h := hosttest.New(t, hosttest.Options{Types: host.Builtins, Width: 190, Height: 48})
	h.Machine.Set(stats)
	h.Advance(2 * time.Second)
	fmt.Printf("=== default layout\n%s\n", h.Screen())
}
