package host_test

// PROTOTYPE for #75, throwaway: prints the screen with the Upgrade popup in
// each placement, under marker B.
// go test ./internal/host -run TestPrototypeUpgradeMarker -v

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/oronbz/visor/internal/hosttest"
)

func TestPrototypeUpgradeMarker(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 110, Height: 24})
	fmt.Println(h.Screen())
	h.Send(tea.KeyPressMsg{Code: tea.KeyF10})
	h.Advance(6 * time.Second)
	fmt.Println(h.Screen())
	h.Send(tea.KeyPressMsg{Code: tea.KeyEscape})
	h.Advance(6 * time.Second)
	fmt.Println(h.Screen())
}
