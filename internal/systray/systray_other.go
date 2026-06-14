//go:build !windows

package systray

// Start is a stub implementation for non-Windows platforms.
func Start(icon []byte, showCb func(), quitCb func()) {
}
