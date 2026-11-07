package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/simtabi/authama/src/internal/brand"
	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/core/renderer"
	"github.com/simtabi/authama/src/internal/platform"
	"github.com/simtabi/authama/src/internal/services/configsvc"
	"github.com/simtabi/authama/src/internal/util/fs"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// Exports are copies for other tools. They are written under the config home,
// never into ~/.ssh, and the split config `config render` maintains is not
// read, rewritten or replaced by either of them.
const (
	flatExportName = "ssh_config"
	stdoutTarget   = "-"
)

func exportDir(p paths.Paths) string { return filepath.Join(p.ConfigDir, "exports") }

func exportPaths(p paths.Paths) renderer.ExportPaths { return renderer.ExportPathsFor(p.SSHDir) }

// exportTarget resolves --out and refuses any target inside the SSH
// directory. An export is a copy for another tool; written into ~/.ssh it
// could replace the split config it was asked to leave alone, the trust
// store, or a key. Containment is decided by inode (os.SameFile) on each
// existing ancestor, not by comparing strings, so neither another spelling on
// a case-insensitive filesystem (~/.SSH/Config on APFS) nor a symlinked
// directory gets past it.
func exportTarget(p paths.Paths, out, defaultName string) (string, error) {
	if out == "" {
		return filepath.Join(exportDir(p), defaultName), nil
	}
	target, err := filepath.Abs(out)
	if err != nil {
		return "", err
	}
	if insideDir(target, p.SSHDir) {
		return "", fmt.Errorf("refusing to write %s: it is inside %s, which `authama ssh config render` and "+
			"the key store own; an export must leave it as it is. Choose an --out outside it", target, p.SSHDir)
	}
	return target, nil
}

// insideDir reports whether path is dir or lies under it, judged by inode:
// each existing ancestor of path is compared with dir by os.SameFile.
func insideDir(path, dir string) bool {
	want, err := os.Stat(dir)
	if err != nil {
		return false // no SSH directory yet: nothing there to overwrite
	}
	for cur := filepath.Clean(path); ; {
		if fi, err := os.Stat(cur); err == nil && os.SameFile(fi, want) {
			return true
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return false
		}
		cur = parent
	}
}

// writeExport writes an export (0600: it names every host and key path) or
// prints it for --out -. It reports where it wrote.
func writeExport(c *cobra.Command, target, text string) error {
	if target == stdoutTarget {
		_, err := fmt.Fprint(c.OutOrStdout(), text)
		return err
	}
	if err := fs.WriteTextAtomic(target, text, 0o600); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(c.OutOrStdout(), "wrote: %s\n", target)
	return nil
}

func loadExport(c *cobra.Command) (paths.Paths, *manifest.Manifest, *configsvc.Service, error) {
	return loadConfigService(c)
}

func newConfigExportCmd() *cobra.Command {
	var flat bool
	var out string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Write the config as one file for another tool (never touches ~/.ssh)",
		Long: "Write a copy of the managed config for a tool that cannot read the split layout.\n\n" +
			"--flat puts every host inline, with absolute key paths and no Include or Match, in\n" +
			"<config home>/exports/ssh_config (or --out, or - for stdout). The files\n" +
			"`config render` writes under ~/.ssh are left exactly as they are: no target inside ~/.ssh is accepted.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if !flat {
				return errors.New("choose a format: --flat (every host inline in one file)")
			}
			p, m, _, err := loadExport(c)
			if err != nil {
				return err
			}
			target := out
			if out != stdoutTarget {
				if target, err = exportTarget(p, out, flatExportName); err != nil {
					return err
				}
			}
			text, err := renderer.RenderFlatConfig(m, exportPaths(p), platform.EmitUseKeychain())
			if err != nil {
				return err
			}
			return writeExport(c, target, text)
		},
	}
	cmd.Flags().BoolVar(&flat, "flat", false, "every host inline in one file, absolute paths, no Include")
	cmd.Flags().StringVarP(&out, "out", "o", "", "where to write (default <config home>/exports/ssh_config; - for stdout)")
	return cmd
}

func newTermiusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "termius",
		Short: "Termius: the key to attach to each host",
		Long: "Termius keeps its hosts in its own encrypted vault, offers no command line or API\n" +
			"to add them, and its SSH-config importer does not follow Include. Write its CSV\n" +
			"import with `" + brand.Command + " export hosts --format termius`; the CSV carries no keys, and\n" +
			"`termius keys` lists and copies the key to attach to each host.",
	}
	cmd.AddCommand(newTermiusKeysCmd())
	return cmd
}
