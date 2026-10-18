// Package version holds the ghud version, stamped at release build time.
package version

// Version is set by GoReleaser with
// -ldflags "-X github.com/oronbz/ghud/internal/version.Version=<tag without v>".
var Version = "dev"
