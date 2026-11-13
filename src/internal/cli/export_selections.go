package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/savedsel"
	"github.com/sangehq/authama/src/internal/services/selection"
)

// newExportSelectionsCmd is `export selections`: the selections saved by
// name, in this account's store and the manifest's.
func newExportSelectionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "selections",
		Short: "List, show, save and delete export selections saved by name",
		Long: "A selection saved by name is exported with `" + brand.Command + " export <name>`. It lives in one of two\n" +
			"stores: user (a file of its own under the state home, private to this account; the default)\n" +
			"or manifest (defaults.export.selections, so it travels with the manifest). The setting\n" +
			"export.save_to changes where a save goes. Both stores are read; on a name in both, the\n" +
			"user's wins.",
		Args: cobra.NoArgs,
	}
	cmd.AddCommand(newExportSelectionsListCmd(), newExportSelectionsShowCmd(), newExportSelectionsSaveCmd(), newExportSelectionsDeleteCmd())
	return cmd
}

// loadForSelections is the manifest, or nil when it cannot be read: the
// user's own store is still listed.
func loadForSelections(c *cobra.Command) (*manifest.Manifest, error) {
	p, err := resolvePaths(c)
	if err != nil {
		return nil, err
	}
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		_, _ = fmt.Fprintf(c.ErrOrStderr(), "the manifest could not be read, so only your own selections are listed: %v\n", err)
		return nil, nil
	}
	return m, nil
}

func newExportSelectionsListCmd() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the saved selections, both stores",
		Example: "  " + brand.Command + " export selections list\n" +
			"  " + brand.Command + " export selections list --json",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			m, err := loadForSelections(c)
			if err != nil {
				return err
			}
			all, err := savedsel.List(p, m)
			if err != nil {
				return err
			}
			out := c.OutOrStdout()
			if asJSON {
				if all == nil {
					all = []savedsel.Saved{}
				}
				b, err := json.MarshalIndent(all, "", "  ")
				if err != nil {
					return err
				}
				_, err = fmt.Fprintln(out, string(b))
				return err
			}
			if len(all) == 0 {
				_, _ = fmt.Fprintf(out, "No saved selections. Save one with `%s export selections save <name> --from <file>`.\n", brand.Command)
				return nil
			}
			tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(tw, "NAME\tSTORE\tROWS\tNOTE")
			for _, s := range all {
				note := ""
				switch {
				case s.Err != "":
					note = "cannot be read: " + s.Err
				case s.Shadowed:
					note = "hidden by your own of the same name"
				}
				grain := s.Selection.Grain
				if grain == "" {
					grain = "-"
				}
				_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", s.Name, s.Store, grain, note)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "print the selections as JSON")
	return cmd
}

func newExportSelectionsShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "show <name>",
		Short:   "Print a saved selection as JSON",
		Example: "  " + brand.Command + " export selections show prd-hosts",
		Args:    cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			m, err := loadForSelections(c)
			if err != nil {
				return err
			}
			s, err := savedsel.Get(p, m, args[0])
			if err != nil {
				return err
			}
			b, err := savedsel.Encode(s.Selection)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.ErrOrStderr(), "%s, from the %s store\n", s.Name, s.Store)
			_, err = fmt.Fprintln(c.OutOrStdout(), string(b))
			return err
		},
	}
}

func newExportSelectionsSaveCmd() *cobra.Command {
	var from, to string
	var yes bool
	cmd := &cobra.Command{
		Use:   "save <name>",
		Short: "Save a selection by name, from a JSON file",
		Long: "Save the selection in --from (a JSON file, such as the Export page's Copy link holds) as\n" +
			"<name>. --to chooses the store: user (yours alone) or manifest (travels with it); without\n" +
			"it, the export.save_to setting does, and that is user unless set. Saving under a name the\n" +
			"store already has replaces it. Export it with `" + brand.Command + " export <name>`.",
		Example: "  " + brand.Command + " export selections save prd-hosts --from sel.json\n" +
			"  " + brand.Command + " export selections save prd-hosts --from sel.json --to manifest",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			if from == "" {
				return fmt.Errorf("give --from <file>, a selection as JSON")
			}
			b, err := os.ReadFile(from) //nolint:gosec // a path the user named
			if err != nil {
				return err
			}
			sel, err := selection.Parse(b)
			if err != nil {
				return err
			}
			store := to
			if store == "" {
				store = savedsel.DefaultStore(nil)
			}
			if err := savedsel.CheckStore(store); err != nil {
				return err
			}
			if store == savedsel.StoreManifest {
				if err := confirmChange(c, "Save "+args[0]+" in the manifest (defaults.export.selections), for every machine that shares it.", yes); err != nil {
					return err
				}
				lockForMutation(p)
				snapshotBeforeMutation(p)
			}
			if err := savedsel.Save(p, store, args[0], sel); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "saved %s (%s rows) in the %s store; export it with `%s export %s`\n",
				args[0], sel.Grain, store, brand.Command, args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "the selection, as a JSON file")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "save to the manifest without confirming (implied when there is no terminal)")
	cmd.Flags().StringVar(&to, "to", "", "the store: "+strings.Join(savedsel.Stores, " or ")+" (default: the export.save_to setting, else user)")
	return cmd
}

func newExportSelectionsDeleteCmd() *cobra.Command {
	var store string
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a saved selection",
		Long: "Delete the saved selection <name> from the store that has it. When both stores have it,\n" +
			"--from says which; the other one stays.",
		Example: "  " + brand.Command + " export selections delete prd-hosts\n" +
			"  " + brand.Command + " export selections delete prd-hosts --from manifest",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			m, err := loadForSelections(c)
			if err != nil {
				return err
			}
			all, err := savedsel.List(p, m)
			if err != nil {
				return err
			}
			var stores []string
			for _, s := range all {
				if s.Name == args[0] {
					stores = append(stores, s.Store)
				}
			}
			switch {
			case store != "":
				if err := savedsel.CheckStore(store); err != nil {
					return err
				}
			case len(stores) == 0:
				return fmt.Errorf("%w named %q", savedsel.ErrNotFound, args[0])
			case len(stores) > 1:
				return fmt.Errorf("both stores have %q: give --from user or --from manifest", args[0])
			default:
				store = stores[0]
			}
			if store == savedsel.StoreManifest {
				if err := confirmChange(c, "Delete "+args[0]+" from the manifest (defaults.export.selections), for every machine that shares it.", yes); err != nil {
					return err
				}
				lockForMutation(p)
				snapshotBeforeMutation(p)
			}
			if err := savedsel.Delete(p, store, args[0]); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(c.OutOrStdout(), "deleted %s from the %s store\n", args[0], store)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "delete from the manifest without confirming (implied when there is no terminal)")
	cmd.Flags().StringVar(&store, "from", "", "the store to delete it from, when both have it: "+strings.Join(savedsel.Stores, " or "))
	return cmd
}
