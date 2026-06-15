//go:build !windows

package winproc

import "os/exec"

// Hide is a no-op on non-Windows platforms.
func Hide(cmd *exec.Cmd) {}
