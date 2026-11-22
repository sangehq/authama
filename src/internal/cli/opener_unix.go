//go:build !windows

package cli

import (
	"os/exec"
	"syscall"
)

// detachOpener puts the opener in a session of its own. A launch from Finder
// runs in a Terminal window whose shell ends as soon as authama does, and the
// hang-up that follows went to the opener too: `open` died before it reached
// the browser, so nothing opened and the link had to be copied by hand.
func detachOpener(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
