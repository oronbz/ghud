//go:build darwin && cgo

package ports

import "testing"

func TestEKAuthorizationStatusMapsToAccess(t *testing.T) {
	for status, want := range map[int]Access{
		0: AccessUndetermined, // notDetermined
		1: AccessDenied,       // restricted
		2: AccessDenied,       // denied
		3: AccessGranted,      // fullAccess
		4: AccessDenied,       // writeOnly can't read events
	} {
		if got := accessOf(status); got != want {
			t.Errorf("EKAuthorizationStatus %d = %v, want %v", status, got, want)
		}
	}
}
