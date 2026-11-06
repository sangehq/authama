// Command authama is the Authentication Access Manager: SSH keys and config
// (`authama ssh ...`) and package-manager / git auth tokens (`authama token
// ...`) in one self-contained Go binary.
//
// The sshmgr alias older releases shipped beside it is gone: `authama ssh
// config render` rewrites a ProxyCommand that still names it, and doctor
// reports a scheduled job whose binary has gone. The end-to-end and
// repository invariant tests live in this package. internal/brand is where the
// names live.
package main

import (
	"os"

	"github.com/simtabi/authama/src/internal/cli"
	"github.com/simtabi/authama/src/internal/util/askpass"
)

func main() {
	// ssh-keygen re-executes this binary as its askpass helper and passes the
	// prompt text as an argument. That is not a verb, so it has to be answered
	// before the command line is parsed.
	if askpass.Serving() {
		os.Exit(askpass.Serve(os.Stdout))
	}
	cli.Execute()
}
