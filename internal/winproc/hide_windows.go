//go:build windows

package winproc

import (
	"os/exec"
	"syscall"
)

// createNoWindow is the Windows CREATE_NO_WINDOW process creation flag. It
// prevents the child process from allocating a console window at all, which is
// what we want for background helpers like the Minecraft java process and the
// playit tunnel agent.
const createNoWindow = 0x08000000

// Hide configures cmd so that, on Windows, the spawned process does not create
// or attach to a console window. Must be called before cmd.Start().
func Hide(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
