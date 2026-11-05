package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/core/renderer"
	"github.com/simtabi/authama/src/internal/util/fs"
)

// newHostExportCmd is `ssh host export <client>`: the hosts as another SSH
// client imports them.
func newHostExportCmd() *cobra.Command {
	var out string
	var includeGit bool
	var skip []string
	var list []string
	for _, n := range renderer.ClientFormatNames() {
		list = append(list, fmt.Sprintf("  %-17s %s", n, renderer.ClientFormats[n].Client))
	}
	cmd := &cobra.Command{
		Use:   "export <client>",
		Short: "Write your hosts as another SSH client imports them",
		Long: "Write every host as another SSH client imports it, into\n" +
			"<config home>/exports/<client>/ (or --out DIR, or - for stdout when the export is\n" +
			"one file). ~/.ssh is never written. The clients:\n\n" +
			strings.Join(list, "\n") + "\n\n" +
			"windows-terminal and iterm2 run `ssh <alias>`, so your ~/.ssh/config supplies the\n" +
			"key and any ProxyCommand. The others are handed a host, port, user and key path,\n" +
			"so a host reached through a ProxyCommand (a VM, Cloudflare Access) is left out.\n" +
			"Git hosts are left out unless --include-git. Termius has its own command,\n" +
			"`ssh termius export`; Tabby, WinSCP and Termius also import the file\n" +
			"`ssh config export --flat` writes.",
		Example: "  " + brand.Command + " ssh host export windows-terminal\n" +
			"  " + brand.Command + " ssh host export putty --skip bastion\n" +
			"  " + brand.Command + " ssh host export csv --out ./client-imports",
		Args:      cobra.ExactArgs(1),
		ValidArgs: renderer.ClientFormatNames(),
		RunE: func(c *cobra.Command, args []string) error {
			format := args[0]
			p, m, _, err := loadExport(c)
			if err != nil {
				return err
			}
			exp, err := renderer.RenderClientExport(m, exportPaths(p), format,
				renderer.TermiusOptions{IncludeGit: includeGit, Skip: skip})
			if err != nil {
				return err
			}
			report := c.OutOrStdout()
			if out == stdoutTarget {
				if len(exp.Files) != 1 {
					return fmt.Errorf("the %s export is %d files; write it to a directory with --out DIR", format, len(exp.Files))
				}
				if _, err := c.OutOrStdout().Write(exp.Files[0].Data); err != nil {
					return err
				}
				report = c.ErrOrStderr()
			} else {
				dir := filepath.Join(exportDir(p), format)
				if out != "" {
					if dir, err = exportTarget(p, out, ""); err != nil {
						return err
					}
				}
				if insideDir(dir, p.SSHDir) {
					return errors.New("refusing to write inside " + p.SSHDir)
				}
				if err := os.MkdirAll(dir, 0o700); err != nil {
					return err
				}
				for _, f := range exp.Files {
					target := filepath.Join(dir, f.Name)
					if err := fs.WriteTextAtomic(target, string(f.Data), 0o600); err != nil {
						return err
					}
					_, _ = fmt.Fprintf(report, "wrote: %s\n", target)
				}
			}
			reportClientExport(report, format, exp)
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "out", "o", "", "the directory to write into (default <config home>/exports/<client>/; - for stdout)")
	cmd.Flags().BoolVar(&includeGit, "include-git", false, "also export git hosts (user git, or a github or gitlab provider), which are not shells")
	cmd.Flags().StringArrayVar(&skip, "skip", nil, "leave out this alias (repeatable)")
	return cmd
}

func reportClientExport(w io.Writer, format string, exp *renderer.ClientExport) {
	_, _ = fmt.Fprintf(w, "%s: %d file(s)\n", format, len(exp.Files))
	for _, s := range exp.Skipped {
		_, _ = fmt.Fprintf(w, "skipped: %s\n", s)
	}
	if len(exp.Notes) > 0 {
		_, _ = fmt.Fprintln(w)
	}
	for _, n := range exp.Notes {
		_, _ = fmt.Fprintln(w, n)
	}
}
