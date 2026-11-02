// Command sshmgr is the deprecated compatibility build of authama (cmd/authama):
// the same program under the name it shipped as before 0.1.0, so existing
// scripts, ProxyCommand lines rendered by older releases (`sshmgr vm proxy`)
// and scheduled notifications keep working. It prints a one-line deprecation
// notice on stderr and then behaves exactly like authama.
package main

import (
	"fmt"
	"os"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/cli"
	"github.com/simtabi/authama/src/internal/util/askpass"
)

func main() {
	// ssh-keygen re-executes this binary as its askpass helper and passes the
	// prompt text as an argument. That is not a verb, so it has to be answered
	// before the command line is parsed - and without the notice, which ssh-keygen
	// would show beside the prompt.
	if askpass.Serving() {
		os.Exit(askpass.Serve(os.Stdout))
	}
	if announce(os.Args[1:]) {
		_, _ = fmt.Fprintln(os.Stderr, brand.DeprecationNotice)
	}
	cli.Execute()
}

// announce reports whether this invocation prints the deprecation notice. It
// does not when nobody would read it as advice: under ssh as a ProxyCommand
// (`vm proxy`), where ssh relays stderr on every connection, and for shell
// completion, which runs on every Tab press or shell start.
func announce(args []string) bool {
	for i, a := range args {
		switch a {
		case "__complete", "__completeNoDesc", "completion":
			return false
		case "proxy":
			if i > 0 && args[i-1] == "vm" {
				return false
			}
		}
	}
	return true
}
