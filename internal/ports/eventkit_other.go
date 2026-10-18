//go:build !darwin || !cgo

package ports

func newEventKit() EventKit { return noEventKit{} }
