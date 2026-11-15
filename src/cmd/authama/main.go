// Command authama is the Authentication Access Manager: SSH keys and config
// (`authama ssh ...`) and package-manager / git auth tokens (`authama token
// ...`) in one self-contained Go binary.
//
// The tool was called ssh-manager and sshmgr before 0.1.1, and an sshmgr
// alias shipped beside authama until after 0.1.3. Neither name is built or
// written now. What older releases left on disk under them is still read
// where losing it would hurt: the managed block in ~/.ssh/config, known_hosts
// pins, provider key titles, bundles, and the old config home for migration. The end-to-end and
// repository invariant tests live in this package. internal/brand is where the
// names live.
package main

import (
	"os"

	"github.com/sangehq/authama/src/internal/cli"
	"github.com/sangehq/authama/src/internal/util/askpass"
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
