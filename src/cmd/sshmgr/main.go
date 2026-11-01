// Command sshmgr is the compatibility build of authama (cmd/authama): the same
// program under the name v1-v3 shipped as, so existing scripts, rendered
// ProxyCommand lines (`sshmgr vm proxy`) and scheduled notifications keep
// working. Its SSH verbs are accepted at the top level as well as under `ssh`.
// The end-to-end and invariant tests live in this package and build it.
package main

import (
	"os"

	"github.com/simtabi/ssh-manager/src/v3/internal/cli"
	"github.com/simtabi/ssh-manager/src/v3/internal/util/askpass"
)

func main() {
	// ssh-keygen re-executes this binary as its askpass helper and passes the
	// prompt text as an argument. That is not a sshmgr verb, so it has to be
	// answered before the command line is parsed.
	if askpass.Serving() {
		os.Exit(askpass.Serve(os.Stdout))
	}
	cli.Execute()
}
