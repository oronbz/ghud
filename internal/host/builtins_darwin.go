//go:build darwin

package host

import (
	"github.com/oronbz/ghud/internal/widget"
	"github.com/oronbz/ghud/internal/widgets/meetings"
)

// macTypes are the built-in Widget types that read EventKit.
var macTypes = []widget.Type{meetings.Type}
