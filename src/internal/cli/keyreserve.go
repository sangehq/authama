package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/services/change"
	"github.com/simtabi/authama/src/internal/services/editor"
)

// newKeyReserveCmd keeps a loose ~/.ssh key on purpose: a standby key for a
// devbox that does not exist yet. doctor then reports it as information, not
// a loose key, and `vm adopt-key`/`vm sync` release the reservation when they
// adopt it. Only the manifest changes; the key file is never touched.
func newKeyReserveCmd() *cobra.Command {
	var forWhat, note string
	var dryRun, yes bool
	cmd := &cobra.Command{
		Use:   "reserve <name|path>",
		Short: "Keep a loose ~/.ssh key on purpose (doctor stops calling it loose)",
		Long: "reserve records that a key file directly in ~/.ssh - one no Host block uses - is kept\n" +
			"deliberately, for example for a devbox VM you have not made yet. doctor then lists it\n" +
			"under \"reserved keys\" as information instead of a loose key (and --strict passes),\n" +
			"and `authama ssh vm adopt-key` or `vm sync` release the reservation when they adopt it.\n\n" +
			"The argument is the file's name (docker-tunnel), ~/.ssh/docker-tunnel, or its .pub.\n" +
			"The reservation lives in manifest.json under reserved_keys, with the public key's\n" +
			"fingerprint, so doctor notices if the file is replaced or removed. The key file itself\n" +
			"is never read beyond its .pub, moved or changed. Reserving a reserved key updates it.",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			name, fp, err := change.ResolveReserve(p, args[0])
			if err != nil {
				return err
			}
			m, err := manifest.Load(p.Manifest())
			if err != nil {
				return err
			}
			verb := "reserve"
			if _, ok := m.ReservedKeys[name]; ok {
				verb = "update the reservation of"
			}
			r := change.ReservationFor(forWhat, note, fp, time.Now())
			summary := fmt.Sprintf("key reserve will %s ~/.ssh/%s (%s)", verb, name, fp)
			if r.For != "" {
				summary += "\n  for:  " + r.For
			}
			if r.Note != "" {
				summary += "\n  note: " + r.Note
			}
			summary += "\n  - only manifest.json changes; the key file is not touched"
			if dryRun {
				_, _ = fmt.Fprintln(c.OutOrStdout(), summary)
				_, _ = fmt.Fprintln(c.OutOrStdout(), "dry run: nothing changed")
				return nil
			}
			if err := confirmChange(c, summary, yes); err != nil {
				return err
			}
			lockForMutation(p)
			if again, _, err := change.ResolveReserve(p, args[0]); err != nil {
				return err
			} else if again != name {
				return errors.New("~/.ssh changed while you were asked; run it again")
			}
			if err := editor.New(p).ReserveKey(name, r); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "reserved ~/.ssh/%s; doctor reports it as kept on purpose\n", name)
			return nil
		},
	}
	f := cmd.Flags()
	f.StringVar(&forWhat, "for", "", "what the key is kept for, e.g. devbox (shown by doctor)")
	f.StringVar(&note, "note", "", "a note to keep with it")
	f.BoolVarP(&dryRun, "dry-run", "n", false, "show what would be recorded; change nothing")
	f.BoolVarP(&yes, "yes", "y", false, "do not ask for confirmation")
	return cmd
}

// newKeyUnreserveCmd drops a reservation: doctor lists the key as loose again.
func newKeyUnreserveCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "unreserve <name>",
		Short: "Drop a key's reservation (doctor lists it as loose again)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			m, err := manifest.Load(p.Manifest())
			if err != nil {
				return err
			}
			name := args[0]
			if _, ok := m.ReservedKeys[name]; !ok {
				return fmt.Errorf("%s is not a reserved key (see `authama ssh key list`)", name)
			}
			if err := confirmChange(c, "key unreserve will drop the reservation of ~/.ssh/"+name+
				"\n  - only manifest.json changes; the key file is not touched", yes); err != nil {
				return err
			}
			lockForMutation(p)
			if _, err := editor.New(p).UnreserveKey(name, false); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "unreserved %s; the key file is untouched\n", name)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "do not ask for confirmation")
	return cmd
}
