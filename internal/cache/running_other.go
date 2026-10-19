//go:build !unix

package cache

// running can't tell here, so a lock lasts until its timeout.
func running(int) bool { return true }
