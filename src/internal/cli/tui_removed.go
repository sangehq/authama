package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/brand"
)

// errTUIRemoved is what `authama tui` answers now that the menu is gone. The
// web UI is the interactive surface, and every menu entry was a CLI command.
var errTUIRemoved = errors.New("the terminal menu was removed: run `" + brand.Command +
	" serve` (or a bare `" + brand.Command + "` on a terminal) for the web UI, " +
	"or `" + brand.Command + " help` for the commands the menu ran")

// newTuiRemovedCmd keeps the name answering, so a script or the atm-ui alias
// from an older install gets the way forward rather than "unknown command".
// It is hidden: nothing should learn it.
func newTuiRemovedCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "tui",
		Short:  "Removed: use the web UI (serve) or the CLI",
		Hidden: true,
		Args:   cobra.ArbitraryArgs,
		RunE:   func(*cobra.Command, []string) error { return errTUIRemoved },
	}
}
