package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/core/sshcfg"
	"github.com/sangehq/authama/src/internal/platform"
	"github.com/sangehq/authama/src/internal/services/configsvc"
	"github.com/sangehq/authama/src/internal/services/knownhosts"
	"github.com/sangehq/authama/src/internal/util/log"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// loadConfigService resolves the home, loads the manifest, and builds the config
// service. emitUseKeychain matches the platform (macOS only), as in v1.
func loadConfigService(c *cobra.Command) (paths.Paths, *manifest.Manifest, *configsvc.Service, error) {
	p, err := resolvePaths(c)
	if err != nil {
		return paths.Paths{}, nil, nil, err
	}
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		return paths.Paths{}, nil, nil, err
	}
	return p, m, configsvc.New(p.SSHDir, m, platform.EmitUseKeychain()).WithState(p.StateDir()), nil
}

// migrateLegacyKnownHosts merges any known_hosts left over under profiles/*/
// from before the trust store consolidated into one file, then deletes them.
// One-shot and idempotent: a tree with none is a no-op, so render can call this
// unconditionally every time rather than requiring a separate migration step.
func migrateLegacyKnownHosts(c *cobra.Command, p paths.Paths, dryRun bool) error {
	legacy, _ := filepath.Glob(filepath.Join(p.SSHDir, "profiles", "*", "known_hosts"))
	if len(legacy) == 0 {
		return nil
	}
	if dryRun {
		_, _ = fmt.Fprintf(c.OutOrStdout(), "would migrate: %d legacy known_hosts file(s) into the single store\n", len(legacy))
		return nil
	}
	snapshotBeforeMutation(p)
	rep, err := knownhosts.New(p.SSHDir).MigrateLegacyStores()
	if err != nil {
		return err
	}
	if len(rep.Removed) > 0 {
		_, _ = fmt.Fprintf(c.OutOrStdout(), "migrated %d known_hosts line(s) from %s into the single store\n",
			rep.Merged, strings.Join(rep.Removed, ", "))
	}
	return nil
}

// newConfigCmd is the first verb group running natively in Go (no engine).
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Render, check, or show the SSH config",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "check",
		Short: "Verify the config matches the manifest (read-only; exit non-zero on drift)",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			_, _, svc, err := loadConfigService(c)
			if err != nil {
				return err
			}
			res, err := svc.Check(true)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(c.OutOrStdout(), res.Format())
			if !res.InSync() {
				return errNotClean
			}
			return nil
		},
	})

	var dryRun, renderYes, force bool
	render := &cobra.Command{
		Use:   "render",
		Short: "Render the config files from the manifest",
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, _, svc, err := loadConfigService(c)
			if err != nil {
				return err
			}
			if !dryRun {
				if err := confirmChange(c,
					"render rewrites the authama block in "+p.SSHDir+
						"/config from the manifest.\nContent outside the markers is preserved.", renderYes); err != nil {
					return err
				}
				// Lock after the question, not before it, so nobody's reading time
				// holds other commands; then read the manifest again under it, so
				// the render is of what is there now.
				lockForMutation(p)
				if p, _, svc, err = loadConfigService(c); err != nil {
					return err
				}
			}
			if err := migrateLegacyKnownHosts(c, p, dryRun); err != nil {
				return err
			}
			if force {
				svc.Force(filepath.Join(p.StateDir(), "config-hand-edits-"+time.Now().Format("20060102-150405")))
			}
			res, err := svc.Write(dryRun)
			if err != nil {
				return err
			}
			verb := "wrote"
			if dryRun {
				verb = "would write"
			}
			out := c.OutOrStdout()
			if len(res.Written) > 0 {
				_, _ = fmt.Fprintf(out, "%s: %s\n", verb, strings.Join(res.Written, ", "))
			}
			if len(res.Pruned) > 0 {
				_, _ = fmt.Fprintf(out, "pruned: %s\n", strings.Join(res.Pruned, ", "))
			}
			if len(res.Written) == 0 && len(res.Pruned) == 0 {
				_, _ = fmt.Fprintln(out, "config: already in sync")
			}
			printCollisions(out, res.Collisions)
			if len(res.HandEdits) > 0 {
				for _, h := range res.HandEdits {
					_, _ = fmt.Fprintf(out, "hand edit: %s\n", h)
				}
				if res.Backup != "" {
					_, _ = fmt.Fprintf(out, "discarded them; the edited files are in %s\n", res.Backup)
				} else if dryRun && !force {
					_, _ = fmt.Fprintln(out, "  -> a render refuses over these: "+configsvc.HandEditFix)
				}
			}
			return nil
		},
	}
	render.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "preview changes without writing")
	render.Flags().BoolVarP(&renderYes, "yes", "y", false, "render without confirming (implied when there is no terminal)")
	render.Flags().BoolVar(&force, "force", false,
		"replace hand edits in the managed files (they are copied to the state home first)")
	cmd.AddCommand(render)

	cmd.AddCommand(&cobra.Command{
		Use:   "show [alias]",
		Short: "Print the rendered config, or ssh -G for one alias",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			_, _, svc, err := loadConfigService(c)
			if err != nil {
				return err
			}
			alias := ""
			if len(args) > 0 {
				alias = args[0]
			}
			out, err := svc.Show(alias)
			_, _ = fmt.Fprint(c.OutOrStdout(), out)
			return err
		},
	})

	cmd.AddCommand(newConfigTreeCmd())
	cmd.AddCommand(newConfigAdoptCmd())
	cmd.AddCommand(newConfigKeepEditsCmd())
	cmd.AddCommand(newConfigExportCmd())
	return cmd
}

// printCollisions warns about hand-written Host blocks that define a managed
// alias again, with the command that resolves them.
func printCollisions(out io.Writer, cs []configsvc.Collision) {
	for _, col := range cs {
		_, _ = fmt.Fprintf(out, "warning: %s\n", col)
	}
	if len(cs) > 0 {
		_, _ = fmt.Fprintln(out, "  -> "+configsvc.CollisionFix)
	}
}

// newConfigAdoptCmd is `ssh config adopt`: take out of ~/.ssh/config the
// hand-written Host blocks that define a managed host again and that the
// managed block already reproduces. It brings a home onboarded before import
// moved the blocks it imported (review R-BUG-09) in line, and is idempotent.
func newConfigAdoptCmd() *cobra.Command {
	var dryRun, yes bool
	cmd := &cobra.Command{
		Use:   "adopt",
		Short: "Move hand-written Host blocks that duplicate managed hosts out of ~/.ssh/config",
		Long: "Finds each hand-written Host block in ~/.ssh/config, outside authama's block, that\n" +
			"defines a host the manifest also renders, and removes the ones whose every line the\n" +
			"managed block already reproduces - so ssh reads one definition of each host and\n" +
			"resolves it exactly as before. A block that sets anything the manifest does not\n" +
			"(another key, a ProxyCommand, a different value) is kept and listed with that line.\n" +
			"The config is snapshotted and copied to the state home first; --dry-run shows the diff.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, _, svc, err := loadConfigService(c)
			if err != nil {
				return err
			}
			if !dryRun {
				if err := confirmChange(c, "adopt removes, from "+filepath.Join(p.SSHDir, "config")+
					", the hand-written Host blocks the manifest already renders the same way.", yes); err != nil {
					return err
				}
				lockForMutation(p)
				if p, _, svc, err = loadConfigService(c); err != nil {
					return err
				}
				snapshotBeforeMutation(p)
			}
			backup := filepath.Join(p.StateDir(), "config-adopt-"+time.Now().Format("20060102-150405"))
			res, err := svc.Adopt(dryRun, backup)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(c.OutOrStdout(), res.Format())
			if !dryRun && len(res.Moved) > 0 {
				aliases := make([]string, 0, len(res.Moved))
				for _, m := range res.Moved {
					aliases = append(aliases, m.Alias)
				}
				log.Audit(p.AuditLog(), "config.adopt", log.Field{Key: "hosts", Value: strings.Join(aliases, ",")},
					log.Field{Key: "backup", Value: res.Backup})
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "show what would be removed, as a diff, without writing")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "adopt without confirming (implied when there is no terminal)")
	return cmd
}

// newConfigTreeCmd is `ssh config tree`: the Include graph of ~/.ssh/config
// and the source file of each Host, read-only. It never writes.
func newConfigTreeCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "tree",
		Short: "Show the Include tree of ~/.ssh/config and the source file of each host (read-only)",
		Long: "Walks ~/.ssh/config the way ssh does for Include directives: relative paths are\n" +
			"resolved against ~/.ssh, globs (*, ?, and ** per path segment) are expanded in\n" +
			"lexical order, Includes nest (inside Host or Match too) up to OpenSSH's limit of 16,\n" +
			"and a cycle or a missing file is a warning, not an error. Each Host is printed with\n" +
			"the file and line it was written on. Authama never rewrites a file it does not\n" +
			"manage; those files are labelled in the tree so you can see what it only reports on.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			tree, err := sshcfg.LoadUser(p.SSHDir)
			if err != nil {
				return err
			}
			if asJSON {
				b, err := tree.JSON()
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintln(c.OutOrStdout(), string(b))
				return nil
			}
			_, _ = fmt.Fprintln(c.OutOrStdout(), tree.Format())
			return nil
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the Include tree as JSON")
	return cmd
}
