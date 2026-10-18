// Package ports holds ghud's outside-world adapters. The host reaches the world
// only through Ports, so the headless test host can swap every one for a fake.
package ports

import (
	"context"
	"net/http"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Ports bundles every outside-world adapter the host app uses.
type Ports struct {
	Clock     Clock
	Opener    Opener
	Clipboard Clipboard
	Editor    Editor
	Keyring   Keyring
	Commands  Commands
	EventKit  EventKit
	Machine   Machine
	HTTP      *http.Client
	Endpoints Endpoints
}

// Clock is the only source of time. Timers go through After so that a fake
// clock can fire them deterministically instead of sleeping.
type Clock interface {
	Now() time.Time
	// After returns a Cmd that delivers msg(t) once d has passed, t being the time it fired.
	After(d time.Duration, msg func(time.Time) tea.Msg) tea.Cmd
}

// Opener hands a URL or path to the system opener (open, xdg-open, rundll32).
type Opener interface {
	Open(target string) error
}

type Clipboard interface {
	Copy(text string) tea.Cmd
}

// Editor hands a file to $EDITOR while the TUI is suspended, then calls
// done. A line above zero opens the file at that line.
type Editor interface {
	Edit(path string, line int, done func(error) tea.Msg) tea.Cmd
}

type Keyring interface {
	Get(service, account string) (string, error)
	Set(service, account, secret string) error
	Delete(service, account string) error
}

// Machine reads how hard this machine is working.
type Machine interface {
	Stats() (Stats, error)
}

type Stats struct {
	// CPU is the share of CPU time in use since the previous read, 0 to 1.
	CPU               float64
	MemUsed, MemTotal uint64
	// Load is the 1, 5 and 15 minute load averages.
	Load [3]float64
}

// Commands runs other CLIs ghud borrows from, such as `gh auth token` and `herdr`.
type Commands interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

type Access int

const (
	AccessUnsupported Access = iota
	AccessUndetermined
	AccessDenied
	AccessGranted
)

type Entity int

const (
	Events Entity = iota
	Reminders
)

// EventKit is the macOS calendar and reminders store. The Meetings and
// Reminders Widget types grow this interface; off darwin it reports AccessUnsupported.
type EventKit interface {
	Status(e Entity) Access
	Request(ctx context.Context, e Entity) (Access, error)
}

// Endpoints holds the base URLs of every HTTP service ghud talks to, so tests
// can point them at httptest servers.
type Endpoints struct {
	GitHubGraphQL string
	GitHubREST    string
	AtlassianAPI  string
	OpenMeteo     string
	Geocoding     string
	IPAPI         string
}

func DefaultEndpoints() Endpoints {
	return Endpoints{
		GitHubGraphQL: "https://api.github.com/graphql",
		GitHubREST:    "https://api.github.com",
		AtlassianAPI:  "https://api.atlassian.com",
		OpenMeteo:     "https://api.open-meteo.com",
		Geocoding:     "https://geocoding-api.open-meteo.com",
		IPAPI:         "https://api.ipapi.is",
	}
}
