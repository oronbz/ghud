package host_test

// PROTOTYPE for #75, throwaway: prints the footer for each marker variant.
// go test ./internal/host -run TestPrototypeUpgradeMarker -v

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/oronbz/visor/internal/hosttest"
)

func TestPrototypeUpgradeMarker(t *testing.T) {
	for _, v := range []string{"A", "B", "C"} {
		var h *hosttest.Host
		for _, w := range []int{140, 80, 64} {
			h = hosttest.New(t, hosttest.Options{Width: w, Height: 20})
			fmt.Printf("%s w=%-3d normal |%s|\n", v, w, h.Line(19))
			h.Press("tab")
			fmt.Printf("%s w=%-3d focus  |%s|\n", v, w, h.Line(19))
		}
		h.Send(tea.KeyPressMsg{Code: tea.KeyF9})
		h.Advance(6 * time.Second)
		fmt.Println()
	}
}
