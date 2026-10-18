package layout_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/oronbz/ghud/internal/layout"
)

func TestSaveWritesWhatLoadReadsAndLeavesOnlyLayoutJSON(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ghud")
	if _, err := layout.Load(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load before any Save: %v, want not-exist", err)
	}
	first := grid([]string{"a", "b"}, []string{"c"}).SetHidden("b", true)
	second := first.Move("a", layout.Right)
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
