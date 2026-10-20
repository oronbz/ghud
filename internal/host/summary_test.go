package host_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/oronbz/visor/internal/theme"
	"github.com/oronbz/visor/internal/widget"
)

// summarised is a 20-Item list under a 6-row summary: half-block digits (3
// rows), a 2-row chart, and a bar line.
func summarised() widget.Payload {
	var titles []string
	for i := 1; i <= 20; i++ {
		titles = append(titles, "i"+strconv.Itoa(i))
	}
	return widget.Payload{
		Lines: []widget.Line{
			{Spans: []widget.Span{{Text: "42", Token: theme.Accent}}, Digits: widget.HalfBlock},
			{Chart: &widget.Chart{Values: []float64{1, 1, 1, 1}, Rows: 2, Token: theme.Ok}},
			{Spans: []widget.Span{{Text: "mem ", Token: theme.Dim}, {Token: theme.Info, Bar: &widget.Bar{Value: 0.5}}}},
		},
		Columns: []widget.Column{{Flex: true}},
		Items:   items("", titles...),
	}
}

func trimmed(line string) string { return strings.Trim(line, "│▐ ") }

func TestAListSummaryIsDrawnAboveTheItemsAndStaysPutWhileTheyScroll(t *testing.T) {
	// 40×16: a 13-row body, 6 rows of summary over 7 Item rows.
	h, _ := withInbox(t, 16, summarised(), nil)
	summary := make([]string, 6)
	for i := range summary {
		summary[i] = h.Line(1 + i)
	}

	if !strings.HasPrefix(summary[0], "│ █ █ ▀▀█ ") {
		t.Errorf("first summary row %q, want the top of big 42 after the margin", summary[0])
	}
	if got := cells(summary[3], 36, 2); got != "⣿⣿" {
		t.Errorf("chart ends %q, want two full braille cells:\n%s", got, h.Screen())
	}
	if !strings.HasPrefix(summary[5], "│ mem ███") {
		t.Errorf("bar line %q, want it after the margin", summary[5])
	}
	if got := trimmed(h.Line(7)); got != "i1" {
		t.Errorf("first Item row shows %q, want i1:\n%s", got, h.Screen())
	}

	h.Press("tab")
	h.Press(strings.Split(strings.Repeat("j", 10), "")...)
	if got := trimmed(h.Line(13)); got != "i11" {
		t.Errorf("selecting i11 shows %q on the last row, want i11:\n%s", got, h.Screen())
	}
	if x, y, ok := h.Find("i11"); !ok || !isSel(h, x, y) {
		t.Errorf("i11 isn't selected and visible:\n%s", h.Screen())
	}
	for i, want := range summary {
		if got := h.Line(1 + i); got != want {
			t.Errorf("summary row %d moved while the list scrolled: %q, want %q", i, got, want)
		}
	}
}

func TestClickingAnItemUnderASummarySelectsItAndClickingTheSummaryDoesNot(t *testing.T) {
	h, _ := withInbox(t, 16, summarised(), nil)
	h.Click(10, 9)
	if x, y, ok := h.Find("i3"); !ok || y != 9 || !isSel(h, x, y) {
		t.Errorf("clicking the row showing i3 didn't select it:\n%s", h.Screen())
	}
	h.Click(10, 2)
	h.Click(10, 6)
	if x, y, _ := h.Find("i3"); !isSel(h, x, y) {
		t.Errorf("clicking summary rows moved the selection off i3:\n%s", h.Screen())
	}

	h.Wheel(10, 9, 5)
	h.Click(10, 7)
	if x, y, ok := h.Find("i6"); !ok || y != 7 || !isSel(h, x, y) {
		t.Errorf("after scrolling, clicking the first Item row didn't select i6:\n%s", h.Screen())
	}
}

func TestATileTooShortForTheSummaryAndOneItemRowShowsOnlyTheList(t *testing.T) {
	// 40×9: a 6-row body, just short of the 6-row summary and one Item row.
	h, _ := withInbox(t, 9, summarised(), nil)
	for y, want := range map[int]string{1: "i1", 6: "i6"} {
		if got := trimmed(h.Line(y)); got != want {
			t.Errorf("row %d shows %q, want %q:\n%s", y, got, want, h.Screen())
		}
	}
	h.Click(10, 2)
	if x, y, ok := h.Find("i2"); !ok || !isSel(h, x, y) {
		t.Errorf("clicking the second row didn't select i2:\n%s", h.Screen())
	}

	// 40×10: a 7-row body fits it all, one Item row under the summary.
	h, _ = withInbox(t, 10, summarised(), nil)
	if got := trimmed(h.Line(7)); got != "i1" {
		t.Errorf("row 7 shows %q, want i1 under the summary:\n%s", got, h.Screen())
	}
}

func TestASummaryChartWithoutAHeightTakesOneRow(t *testing.T) {
	p := summarised()
	p.Lines = []widget.Line{{Chart: &widget.Chart{Values: []float64{1, 1}, Token: theme.Ok}}}
	h, _ := withInbox(t, 16, p, nil)
	if got := cells(h.Line(1), 37, 1); got != "⣿" {
		t.Errorf("chart row ends %q, want a full braille cell:\n%s", got, h.Screen())
	}
	if got := trimmed(h.Line(2)); got != "i1" {
		t.Errorf("row 2 shows %q, want i1 right under the 1-row chart:\n%s", got, h.Screen())
	}
}

func TestBarsInItemFieldsAreDrawnAndSizeTheirColumns(t *testing.T) {
	half := func(width int) widget.Span {
		return widget.Span{Token: theme.Info, Bar: &widget.Bar{Value: 0.5, Width: width}}
	}
	p := widget.Payload{
		Columns: []widget.Column{{Flex: true}, {}, {}},
		Items: []widget.Item{
			{ID: "cpu", Fields: []widget.Field{{{Text: "cpu ", Token: theme.Fg}, half(0)}, {half(6)}, {half(0)}}},
			{ID: "mem", Fields: []widget.Field{widget.F(theme.Fg, "mem"), nil, widget.F(theme.Dim, "35%")}},
		},
	}
	h, _ := withInbox(t, 8, p, nil)

	// 36 cells: a 6-cell bar, a 10-cell bar, two gaps, and 18 for the flex column.
	want := "│ cpu " + strings.Repeat("█", 7) + strings.Repeat("─", 7) + " ███─── █████───── │"
	if got := h.Line(1); got != want {
		t.Errorf("bars row\n%q, want\n%q", got, want)
	}
	if got, want := h.Line(2), "│ mem"+strings.Repeat(" ", 23)+"35%        │"; got != want {
		t.Errorf("text row\n%q, want\n%q", got, want)
	}

	h.Press("tab")
	if fg, _ := h.Fg(2+4, 1); fg != theme.Info {
		t.Errorf("selected bar drawn in %v, want its own info", fg)
	}
	if bg, _ := h.Bg(2+4+10, 1); bg != theme.Sel {
		t.Errorf("selected bar's track background %v, want sel", bg)
	}
}

func TestAnEmptyListWithASummaryShowsTheSummaryThenTheEmptyText(t *testing.T) {
	p := summarised()
	p.Items, p.Empty = nil, "nothing to review"
	h, _ := withInbox(t, 16, p, nil)
	if !strings.HasPrefix(h.Line(1), "│ █ █ ▀▀█ ") {
		t.Errorf("row 1 %q, want the summary's digits", h.Line(1))
	}
	if got := trimmed(h.Line(7)); got != "nothing to review" {
		t.Errorf("row 7 shows %q, want the empty text under the summary:\n%s", got, h.Screen())
	}
}
