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
	Watcher   Watcher
	Inbox     Inbox
	HTTP      *http.Client
	Endpoints Endpoints
	// Getenv reads the process environment, such as GH_TOKEN.
	Getenv func(key string) string
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

// Watcher reports changes to files on disk, whoever made them: another
// ghud instance, an editor, or this instance itself.
type Watcher interface {
	// Watch replaces the set of watched files. A file that doesn't exist
	// yet is reported when it's created.
	Watch(files []string) error
	// Next returns a Cmd that waits for the next change and delivers
	// msg(files changed), each spelt as it was given to Watch.
	Next(msg func(files []string) tea.Msg) tea.Cmd
}

// Inbox turns messages that arrive on their own, from goroutines outside the
// program such as a Plugin's reader, into Cmds.
type Inbox interface {
	// Next returns a Cmd that waits for the next message on ch.
	Next(ch <-chan tea.Msg) tea.Cmd
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

// EventKit is the macOS calendar and reminders store. Off darwin it reports
// AccessUnsupported.
type EventKit interface {
	// Status reads e's access without ever asking the user.
	Status(e Entity) Access
	// Request asks the user for full access to e when they haven't answered
	// yet, blocking until they do; otherwise it reports the access as it is.
	Request(ctx context.Context, e Entity) (Access, error)
	// Events are the events from every calendar that overlap [from, to).
	// Without granted access it fails rather than asking.
	Events(ctx context.Context, from, to time.Time) ([]Event, error)
	// Reminders are the reminders not yet completed, from every list.
	// Without granted access it fails rather than asking.
	Reminders(ctx context.Context) ([]Reminder, error)
	// Complete marks reminder id completed and saves it.
	Complete(ctx context.Context, id string) error
}

// Reminder is one open reminder.
type Reminder struct {
	ID    string
	Title string
	List  string
	// Due is zero when the reminder has no due date.
	Due time.Time
	// DateOnly is set when Due is a day without a time of day.
	DateOnly bool
}

// Event is one occurrence of a calendar event.
type Event struct {
	// ID is the event's EventKit identifier; occurrences of a recurring
	// event share it.
	ID         string
	Title      string
	Start, End time.Time
	AllDay     bool
	Location   string
	Calendar   string
	// Declined is set when the user declined the invitation.
	Declined bool
	Canceled bool
}

// Endpoints holds the base URLs of every HTTP service ghud talks to, so tests
// can point them at httptest servers.
type Endpoints struct {
	GitHubGraphQL string
	GitHubREST    string
	AtlassianAPI  string
	// Jira, when set, stands in for every Jira site's URL, so tests reach a
	// fake Jira whatever `site` says. It's empty for real use.
	Jira      string
	OpenMeteo string
	Geocoding string
	IPAPI     string
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
