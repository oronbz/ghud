// Package version holds the visor version, stamped at release build time.
package version

// Version is set by GoReleaser with
// -ldflags "-X github.com/oronbz/visor/internal/version.Version=<tag without v>".
var Version = "dev"
