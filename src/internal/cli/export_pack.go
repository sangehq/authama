package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/services/transfer"
	"github.com/sangehq/authama/src/internal/util/encode"
	"github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/version"
)

// newExportPackCmd is `export pack`: the web UI's Export / Import page at a
// terminal. It zips the parts chosen; no part holds a secret.
func newExportPackCmd() *cobra.Command {
	var include, clients []string
	var termiusGroups, out string
	var includeGit, dryRun bool
	var long strings.Builder
	long.WriteString("Zip the files the home exports as, chosen by part: the same pack the web UI's\n" +
		"Export / Import page downloads. No part holds a private key or a token value; the encrypted\n" +
		"full export is `" + brand.Command + " setup export --encrypt`.\n\nParts (--include, comma-separated, or all):\n")
	for _, p := range transfer.Parts {
		fmt.Fprintf(&long, "  %-12s %s (%s)\n", p.ID, p.Title, p.Files)
	}
	long.WriteString("\nWith no --include the pack holds " + strings.Join(transfer.Default, ", ") + ".\n" +
		"The zip goes to <config home>/exports/" + brand.Command + "-export-<time>.zip, or --out (never inside\n" +
		"~/.ssh). --dry-run lists the files and their sizes and writes nothing. `setup import` takes the\n" +
		"zip as it takes setup.json.")
	cmd := &cobra.Command{
		Use:   "pack",
		Short: "Zip any of the exports - Termius hosts, ssh config, setup, structure, git signing, lists - in one file",
		Long:  long.String(),
		Example: "  " + brand.Command + " export pack\n" +
			"  " + brand.Command + " export pack --include termius,ssh-config --termius-groups category/profile --out hosts.zip\n" +
			"  " + brand.Command + " export pack --include all --dry-run\n" +
			"  " + brand.Command + " export pack --include ssh-clients --clients tabby,filezilla,ansible",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			now := time.Now()
			files, err := transfer.Build(p, transfer.Options{Include: include, TermiusGroups: termiusGroups,
				IncludeGit: includeGit, Clients: clients, Version: version.Version, Now: now})
			if err != nil {
				return err
			}
			w := c.OutOrStdout()
			total := 0
			for _, f := range files {
				total += len(f.Body)
				if dryRun {
					_, _ = fmt.Fprintf(w, "%8d  %s\n", len(f.Body), f.Name)
				}
			}
			if dryRun {
				_, _ = fmt.Fprintf(w, "%d file(s), %d bytes; nothing written\n", len(files), total)
				return nil
			}
			data, err := transfer.Zip(files, now)
			if err != nil {
				return err
			}
			target, err := exportTarget(p, out, brand.Command+"-export-"+now.Format("20060102-150405")+".zip")
			if err != nil {
				return err
			}
			if err := fs.WriteTextAtomic(target, string(data), 0o600); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(w, "wrote %s: %s\n", target, strings.Join(transfer.Names(files), ", "))
			return nil
		},
	}
	f := cmd.Flags()
	f.StringSliceVar(&include, "include", nil, "the parts: "+strings.Join(transfer.IDs(), ", ")+", or all")
	f.StringVar(&termiusGroups, "termius-groups", encode.TermiusGroupsProfile, "termius: the Groups column: "+strings.Join(encode.TermiusGroupSchemes, ", "))
	f.BoolVar(&includeGit, "include-git", false, "termius: also list git hosts")
	f.StringSliceVar(&clients, "clients", nil, "ssh-clients: only these client formats ("+strings.Join(transfer.ClientFormats, ", ")+"); every one by default")
	f.StringVarP(&out, "out", "o", "", "where to write the zip (mode 0600)")
	f.BoolVar(&dryRun, "dry-run", false, "list the files and sizes; write nothing")
	return cmd
}
