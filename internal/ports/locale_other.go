//go:build !darwin || !cgo

package ports

func systemLocale() SystemLocale { return SystemLocale{} }
