//go:build !windows

package autostart

// Set is a stub implementation for non-Windows platforms.
func Set(enabled bool) error {
	return nil
}
