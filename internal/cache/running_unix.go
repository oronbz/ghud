//go:build unix

package cache

import (
	"errors"
	"syscall"
)

// running is whether process pid exists; one owned by another user counts.
func running(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
