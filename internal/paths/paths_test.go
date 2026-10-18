package paths_test

import (
	"path/filepath"
	"testing"

	"github.com/oronbz/ghud/internal/paths"
)

func TestResolve(t *testing.T) {
	j := filepath.Join
	tests := []struct {
		name string
		goos string
		env  map[string]string
		want paths.Paths
	}{
		{
			name: "home defaults on macOS, not ~/Library",
			goos: "darwin",
			env:  map[string]string{"HOME": "/Users/me"},
			want: paths.Paths{Config: j("/Users/me", ".config", "ghud"), Data: j("/Users/me", ".local", "share", "ghud"), Cache: j("/Users/me", ".cache", "ghud")},
		},
		{
			name: "XDG variables win",
			goos: "darwin",
			env:  map[string]string{"HOME": "/Users/me", "XDG_CONFIG_HOME": "/x/conf", "XDG_DATA_HOME": "/x/data", "XDG_CACHE_HOME": "/x/cache"},
			want: paths.Paths{Config: j("/x/conf", "ghud"), Data: j("/x/data", "ghud"), Cache: j("/x/cache", "ghud")},
		},
		{
			name: "relative XDG values are ignored, per the XDG spec",
			goos: "linux",
			env:  map[string]string{"HOME": "/home/me", "XDG_CONFIG_HOME": "rel"},
			want: paths.Paths{Config: j("/home/me", ".config", "ghud"), Data: j("/home/me", ".local", "share", "ghud"), Cache: j("/home/me", ".cache", "ghud")},
		},
		{
			name: "Windows uses AppData and LocalAppData",
			goos: "windows",
			env:  map[string]string{"AppData": "/w/roaming", "LocalAppData": "/w/local"},
			want: paths.Paths{Config: j("/w/roaming", "ghud"), Data: j("/w/local", "ghud", "data"), Cache: j("/w/local", "ghud", "cache")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := paths.Resolve(func(k string) string { return tt.env[k] }, tt.goos)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestResolveWithoutHomeFails(t *testing.T) {
	if _, err := paths.Resolve(func(string) string { return "" }, "linux"); err == nil {
		t.Error("resolved paths with no $HOME")
	}
}
