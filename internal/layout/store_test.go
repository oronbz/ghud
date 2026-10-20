package layout_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/oronbz/visor/internal/layout"
)

func TestSaveWritesWhatLoadReadsAndLeavesOnlyLayoutJSON(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "visor")
	if _, err := layout.Load(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load before any Save: %v, want not-exist", err)
	}
	first := grid([]string{"a", "b"}, []string{"c"}).SetHidden("b", true)
	second, _ := first.Move("a", layout.Right, area, layout.Via{})
	for _, l := range []layout.Layout{first, second} {
		if err := layout.Save(dir, l); err != nil {
			t.Fatal(err)
		}
		got, err := layout.Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, l) {
			t.Errorf("Load after Save = %+v, want %+v", got, l)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != layout.FileName {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("config dir holds %v, want only %s", names, layout.FileName)
	}
}

func TestTodaysLayoutFileLoadsAndSavesBackByteForByte(t *testing.T) {
	const today = `{
  "default": {
    "rows": [
      {
        "weight": 12,
        "widgets": [
          {
            "id": "clock",
            "weight": 3
          },
          {
            "id": "calendar",
            "weight": 3.5
          }
        ]
      },
      {
        "weight": 10,
        "widgets": [
          {
            "id": "prs",
            "weight": 1
          }
        ]
      }
    ],
    "hidden": [
      "calendar"
    ]
  }
}
`
	roundTrip(t, today)
}

func TestAStackSavesAsACellWithRows(t *testing.T) {
	const stacked = `{
  "default": {
    "rows": [
      {
        "weight": 1,
        "widgets": [
          {
            "id": "priorities",
            "weight": 3
          },
          {
            "weight": 7,
            "rows": [
              {
                "weight": 1,
                "widgets": [
                  {
                    "id": "spotify",
                    "weight": 1
                  }
                ]
              },
              {
                "weight": 2,
                "widgets": [
                  {
                    "id": "agents",
                    "weight": 1
                  }
                ]
              }
            ]
          }
        ]
      }
    ]
  }
}
`
	l := roundTrip(t, stacked)
	if s := l.Rows[0].Cells[1]; !s.IsStack() || len(s.Rows) != 2 || s.Rows[1].Cells[0].ID != "agents" {
		t.Errorf("second cell %+v, want a stack of spotify over agents", s)
	}
	if l.Rows[0].Cells[0].IsStack() {
		t.Error("a Widget cell reads as a stack")
	}
}

// roundTrip loads text as layout.json and saves it again, failing unless the
// file comes back unchanged.
func roundTrip(t *testing.T, text string) layout.Layout {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, layout.FileName)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := layout.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.Save(dir, l); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != text {
		t.Errorf("saved back as:\n%s\nwant:\n%s", b, text)
	}
	return l
}
