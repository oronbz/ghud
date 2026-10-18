package ports

import (
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
)

type gopsutilMachine struct{}

func newMachine() gopsutilMachine {
	// The first non-blocking read is the average since boot; spend it here so
	// the first Stats covers only the time since launch.
	_, _ = cpu.Percent(0, false)
	return gopsutilMachine{}
}

func (gopsutilMachine) Stats() (Stats, error) {
	var s Stats
	p, err := cpu.Percent(0, false)
	if err != nil {
		return s, err
	}
	if len(p) > 0 {
		s.CPU = p[0] / 100
	}
	v, err := mem.VirtualMemory()
	if err != nil {
		return s, err
	}
	s.MemUsed, s.MemTotal = v.Used, v.Total
	// Load averages are approximate on Windows and never block the reading.
	if l, err := load.Avg(); err == nil {
		s.Load = [3]float64{l.Load1, l.Load5, l.Load15}
	}
	return s, nil
}
