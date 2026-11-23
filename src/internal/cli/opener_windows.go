//go:build windows

package cli

import (
	"os/exec"
	"syscall"
)

// detachOpener starts the opener in a process group of its own, so closing
// the console window authama ran in does not take it down with it.
func detachOpener(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200} // CREATE_NEW_PROCESS_GROUP
}
