package host_test

import (
	"context"
	"errors"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/widget"
)

// board is a list Widget whose one card moves between columns through a
// choice Action; the columns it offers load like they would from a network.
type board struct{ log *boardLog }

type boardLog struct {
	mu        sync.Mutex
	choices   []widget.Choice
	err       error
	asked     []widget.Invocation
	ran       []widget.Invocation
	refreshes int
	// during, when set, runs inside Choices, while the choices load.
	during func()
}

func (b board) Refresh(context.Context, time.Time) (widget.Payload, error) {
	b.log.mu.Lock()
	b.log.refreshes++
	b.log.mu.Unlock()
	return widget.Payload{
		Columns: []widget.Column{{Flex: true}},
		Items: []widget.Item{{ID: "card-1", Fields: []widget.Field{widget.F(0, "Fix the login page")}, Actions: []widget.Action{
			{ID: "move", Key: "t", Label: "move", Input: widget.ChoiceInput, Prompt: "Move to"},
		}}},
	}, nil
}

func (b board) Choices(_ context.Context, inv widget.Invocation) ([]widget.Choice, error) {
	b.log.mu.Lock()
	b.log.asked = append(b.log.asked, inv)
	during := b.log.during
	b.log.mu.Unlock()
	if during != nil {
		during()
	}
	return b.log.choices, b.log.err
}

func (b board) Invoke(_ context.Context, inv widget.Invocation) (widget.Result, error) {
	b.log.mu.Lock()
	defer b.log.mu.Unlock()
	b.log.ran = append(b.log.ran, inv)
	return widget.Result{Toast: "Moved card-1"}, nil
}

func withBoard(t *testing.T, log *boardLog) *hosttest.Host {
	t.Helper()
	typ := widget.Type{Name: "board", Title: "Board", New: func(widget.Config) widget.Widget { return board{log: log} }}
	return hosttest.New(t, hosttest.Options{Width: 60, Height: 14, Types: []widget.Type{typ}})
}

var spinnerGlyph = regexp.MustCompile(`[⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏]`)

func TestAChoiceActionOpensASpinnerMenuAtOnceThenItsChoicesAndRunsThePickedOne(t *testing.T) {
	log := &boardLog{choices: []widget.Choice{{ID: "11", Label: "To Do"}, {ID: "21", Label: "In Progress"}, {ID: "31", Label: "Done"}}}
	h := withBoard(t, log)
	var during string
	log.during = func() { during = h.Screen() }

	h.Press("1", "t")
	if during == "" {
		t.Fatal("t didn't load the choices")
	}
	if !strings.Contains(during, "Move to") || !spinnerGlyph.MatchString(during) {
		t.Errorf("while the choices load, want a Move to menu with a spinner:\n%s", during)
	}
	if lines := strings.Split(during, "\n"); !strings.Contains(lines[len(lines)-1], "loading") {
		t.Errorf("while the choices load, the footer reads %q, want it to say so", lines[len(lines)-1])
	}
	if strings.Contains(during, "In Progress") {
		t.Errorf("choices shown before they loaded:\n%s", during)
	}
	if got := log.asked; len(got) != 1 || got[0].Action != "move" || got[0].Item != "card-1" {
		t.Errorf("asked for choices with %+v, want the move Action on card-1", got)
	}

	if got, want := menuEntries(h), []string{"To Do", "In Progress", "Done"}; !containsRun(got, want) {
		t.Fatalf("menu entries %q, want %q", got, want)
	}
	if spinnerGlyph.MatchString(h.Screen()) {
		t.Errorf("spinner left once the choices loaded:\n%s", h.Screen())
	}

	before := log.refreshes
	h.Press("j", "enter")
	if got := log.ran; len(got) != 1 || got[0].Action != "move" || got[0].Item != "card-1" || got[0].Choice != "21" {
		t.Fatalf("ran %+v, want move on card-1 with choice 21", got)
	}
	if !strings.Contains(h.Line(13), "Moved card-1") {
		t.Errorf("footer %q, want the Action's toast", h.Line(13))
	}
	if log.refreshes != before+1 {
		t.Errorf("refreshed %d times after the Action, want once", log.refreshes-before)
	}
	if strings.Contains(h.Screen(), "In Progress") {
		t.Errorf("menu still open after choosing:\n%s", h.Screen())
	}
}

func TestAChoiceWithATargetOpensItInsteadOfRunningTheAction(t *testing.T) {
	log := &boardLog{choices: []widget.Choice{{ID: "41", Label: "Resolve", Target: "https://example.com/card-1"}}}
	h := withBoard(t, log)

	h.Press("1", "t", "enter")
	if got := h.Opener.Opened; len(got) != 1 || got[0] != "https://example.com/card-1" {
		t.Errorf("opened %v, want the choice's target", got)
	}
	if len(log.ran) != 0 {
		t.Errorf("ran %+v, want nothing run for a choice with a target", log.ran)
	}
}

func TestChoicesThatFailToLoadCloseTheMenuWithAToast(t *testing.T) {
	log := &boardLog{err: errors.New("Jira is down")}
	h := withBoard(t, log)

	h.Press("1", "t")
	if !strings.Contains(h.Line(13), "Move failed: Jira is down") {
		t.Errorf("footer %q, want the load error", h.Line(13))
	}
	if strings.Contains(h.Screen(), "Move to") {
		t.Errorf("menu still open after the choices failed:\n%s", h.Screen())
	}
	h.Press("q")
	if !h.Quit() {
		t.Error("q didn't quit after the menu closed")
	}
}

func TestNoChoicesSaysSoAndClosesTheMenu(t *testing.T) {
	h := withBoard(t, &boardLog{})

	h.Press("1", "t")
	if !strings.Contains(h.Line(13), "Nothing to move to") {
		t.Errorf("footer %q, want a nothing-to-choose toast", h.Line(13))
	}
	if strings.Contains(h.Screen(), "Move to") {
		t.Errorf("menu still open with no choices:\n%s", h.Screen())
	}
}

func TestChoicesArrivingAfterTheMenuClosedAreIgnored(t *testing.T) {
	log := &boardLog{choices: []widget.Choice{{ID: "21", Label: "In Progress"}}}
	h := withBoard(t, log)
	log.during = func() { h.Press("esc") }

	h.Press("1", "t")
	if strings.Contains(h.Screen(), "In Progress") {
		t.Errorf("a closed menu reopened when its choices arrived:\n%s", h.Screen())
	}
}

// containsRun reports whether want appears in got as consecutive entries.
func containsRun(got, want []string) bool {
	for i := range got {
		if len(got)-i >= len(want) && slices.Equal(got[i:i+len(want)], want) {
			return true
		}
	}
	return false
}
