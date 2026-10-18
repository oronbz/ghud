// Package widget is the contract between the host and Widget types. Built-in
// Widget types implement it directly; the external plugin host adapts
// JSON-RPC to it. Widgets describe what to show; the host draws it (ADR 0001).
package widget

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/oronbz/ghud/internal/ports"
	"github.com/oronbz/ghud/internal/theme"
)

// Type is a kind of Widget, placed on the dashboard as one or more Widgets.
type Type struct {
	// Name is the `type` key in config.toml.
	Name string
	// Title is the default title drawn in the Widget's top border.
	Title string
	// Description is the one-line summary above the type's block in the config reference.
	Description string
	// Settings is the schema a Widget's config.toml block is validated
	// against and the config reference is generated from.
	Settings []Setting
	// Refresh is how often the host refreshes a Source; zero means only at
	// start and after its Actions.
	Refresh time.Duration
	New     func(Config) Widget
}

// Config is what a Widget is built from: its validated settings, defaults
// filled in, and the global options that affect it.
type Config struct {
	Settings Settings
	// TimeFormat is "12h" or "24h".
	TimeFormat string
	// WeekStart is a lower-case English weekday, such as "sunday".
	WeekStart string
	// DataDir is ghud's data directory, where local Widget data lives by default.
	DataDir string
	// Ports are the outside-world adapters, the only way a Widget reaches the world.
	Ports ports.Ports
}

// Widget is one placed Widget. It is a View, drawn afresh for every frame,
// or a Source, whose Payload the host fetches and keeps; either may also be
// an Actor.
type Widget any

// View is a live view such as the Clock: its payload depends only on the frame.
type View interface {
	Payload(Frame) Payload
}

// Ticker is a View that changes with time, such as the Clock every second.
type Ticker interface {
	// Next is when the View next changes after now; the host redraws then.
	Next(now time.Time) time.Time
}

// Source is a Widget whose data lives outside ghud, such as in a file or
// behind an API. The host calls Refresh off the UI thread and draws the last
// Payload it returned.
type Source interface {
	Refresh(ctx context.Context, now time.Time) (Payload, error)
}

// Watcher is a Source backed by local files. Widgets sharing a file refresh
// together after an Action on any of them.
type Watcher interface {
	Files() []string
}

// Actor runs the Actions a Widget declares in its Payload. Invoke runs off
// the UI thread; the Item's new state arrives with the next Refresh.
type Actor interface {
	Invoke(ctx context.Context, inv Invocation) (Result, error)
}

// Invocation is one run of an Action.
type Invocation struct {
	// Action is the Action's ID.
	Action string
	// Item is the selected Item's ID, or "" for a Widget Action.
	Item string
	// Text is what the user typed into a text input.
	Text string
	Now  time.Time
}

// Result is what a successful Action reports back.
type Result struct {
	// Toast is shown in the footer.
	Toast string
}

type Size struct{ W, H int }

// Frame is what the host knows when it asks a Widget for its payload.
type Frame struct {
	Now time.Time
	// Size is the tile's inner size, border excluded.
	Size    Size
	Focused bool
}

// Payload is what a Widget shows: a live view's styled Lines, or a list
// Widget's Items under Columns. Either kind may carry Widget Actions.
type Payload struct {
	Lines []Line
	// Center centres the block vertically and each line horizontally.
	Center bool

	// Columns make the Payload a list: every Item has one Field per Column.
	Columns []Column
	Items   []Item
	// Count is drawn after a list Widget's title, such as the number of open tasks.
	Count int
	// Empty is shown when the list has no Items.
	Empty string

	// Actions apply to the whole Widget rather than to an Item.
	Actions []Action
}

// IsList reports whether the Payload is a list of Items rather than lines.
func (p Payload) IsList() bool { return len(p.Columns) > 0 }

// Column is one column of a list.
type Column struct {
	// Priority decides which columns survive a narrow Widget: the
	// highest-numbered column is dropped first.
	Priority int
	// Flex columns share the width the others leave and truncate to fit;
	// the others are as wide as their widest Field.
	Flex bool
	// Right aligns the column's Fields to its right edge.
	Right bool
}

// Item is one row of a list Widget.
type Item struct {
	// ID is stable across refreshes, so the selection follows the Item.
	ID string
	// Group, when set, puts the Item under a header; consecutive Items with
	// the same Group share one.
	Group   string
	Fields  []Field
	Actions []Action
}

// Field is one cell of an Item, in its Column.
type Field []Span

// F is a Field of one span.
func F(tok theme.Token, text string) Field { return Field{{Text: text, Token: tok}} }

// Action is something the user can do to an Item or to the whole Widget.
type Action struct {
	// ID is free-form; the Widget gets it back in Invocation.Action.
	ID string
	// Key runs the Action while its Widget is focused, written the way Bubble
	// Tea prints keys ("a", "enter", "space"). Host keys win over it.
	Key   string
	Label string
	// Primary runs on enter, whatever its Key.
	Primary bool
	// Confirm asks before running, for Actions that can't be taken back.
	Confirm bool
	Verb    Verb
	// Target is the URL an Open Action opens or the text a Copy Action
	// copies; the host performs both itself.
	Target string
	Input  Input
	// Prompt titles a text input's prompt; Value is the text it starts with.
	Prompt, Value string
}

// Verb says what kind of thing an Action does.
type Verb string

const (
	Open     Verb = "open"
	Copy     Verb = "copy"
	Complete Verb = "complete"
	Delete   Verb = "delete"
	Edit     Verb = "edit"
)

// Input is what the host asks for before running an Action.
type Input int

const (
	NoInput Input = iota
	// TextInput asks for one line in the host prompt.
	TextInput
)

type Line struct {
	Spans []Span `json:"spans,omitempty"`
	// Digits asks the host to draw the line's text as big digits.
	Digits Digits `json:"digits,omitempty"`
	// Chart, when set, draws the line as a braille area chart instead of its Spans.
	Chart *Chart `json:"chart,omitempty"`
}

// Chart is a braille area chart, two samples per cell and four dots per row.
type Chart struct {
	// Values are samples from 0 to 1, oldest first; the newest is drawn at the
	// right edge and the ones that don't fit are dropped.
	Values []float64 `json:"values"`
	// Rows is the chart's height; zero takes the rows the other lines leave.
	Rows  int         `json:"rows,omitempty"`
	Token theme.Token `json:"token"`
}

// Digits is how big the host draws a line's text. Big digits cover 0-9,
// ':', space, A, P and M; other characters are skipped.
type Digits int

const (
	NoDigits Digits = iota
	// HalfBlock digits are 3 rows tall.
	HalfBlock
	// Block digits are 5 rows tall and twice as wide as half-block ones.
	Block
)

var digitNames = []string{"", "half-block", "block"}

func (d Digits) MarshalText() ([]byte, error) {
	if d < 0 || int(d) >= len(digitNames) {
		return nil, fmt.Errorf("unknown digits %d", d)
	}
	return []byte(digitNames[d]), nil
}

func (d *Digits) UnmarshalText(b []byte) error {
	i := slices.Index(digitNames, string(b))
	if i < 0 {
		return fmt.Errorf("digits %q must be \"half-block\" or \"block\"", b)
	}
	*d = Digits(i)
	return nil
}

type Span struct {
	Text  string      `json:"text,omitempty"`
	Token theme.Token `json:"token"`
	// Bg fills the span's cells; the zero value, theme.Bg, leaves the background alone.
	Bg   theme.Token `json:"bg,omitempty"`
	Bold bool        `json:"bold,omitempty"`
	// Bar, when set, draws the span as an eighth-block bar instead of its Text.
	Bar *Bar `json:"bar,omitempty"`
}

// Bar is a horizontal bar in the span's Token over a track in the line token.
type Bar struct {
	// Value is the filled share, 0 to 1.
	Value float64 `json:"value"`
	// Width is the bar's width in cells; zero takes the width the line's other spans leave.
	Width int `json:"width,omitempty"`
}

func Text(tok theme.Token, s string) Line { return Line{Spans: []Span{{Text: s, Token: tok}}} }
