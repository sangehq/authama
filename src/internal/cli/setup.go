package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/platform"
	"github.com/sangehq/authama/src/internal/services/bundler"
	"github.com/sangehq/authama/src/internal/services/change"
	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/services/keystore"
	"github.com/sangehq/authama/src/internal/services/setupxfer"
	"github.com/sangehq/authama/src/internal/util/agepass"
	"github.com/sangehq/authama/src/internal/util/fs"
	"github.com/sangehq/authama/src/internal/util/log"
	"github.com/sangehq/authama/src/internal/util/paths"
	"github.com/sangehq/authama/src/internal/version"
)

func newSetupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Export this setup to another machine, or import one (secrets-free, or encrypted with keys and tokens)",
		Long: "Move a whole setup between machines.\n\n" +
			"`setup export` writes a setup document: the manifest, providers, settings (without\n" +
			"this machine's paths) and token config - the names of the variables tokens come from,\n" +
			"never a value. It is JSON, safe to read and to keep in a repository.\n\n" +
			"`setup export --encrypt` writes a full export: the same, plus every private key, the\n" +
			"inventory and the token values, in an age file encrypted with a passphrase (`age -d`\n" +
			"opens it too). It needs the owner PIN when one is set.\n\n" +
			"`setup import` reads either, shows what it would add, keep and replace, and merges it\n" +
			"into this home. --on-conflict settles what differs: keep (the default) leaves what is\n" +
			"here, replace takes the import's - keeping a copy of every key file and .env it\n" +
			"replaces in the state home's replaced/ folder - and fail writes nothing.",
	}
	cmd.AddCommand(newSetupExportCmd(), newSetupImportCmd())
	return cmd
}

func newSetupExportCmd() *cobra.Command {
	var out, passFile string
	var encrypt bool
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Write this setup as a secrets-free document, or (--encrypt) everything, keys and tokens included",
		Example: "  " + brand.Command + " setup export --out setup.json\n" +
			"  " + brand.Command + " setup export --encrypt --out ~/Desktop/setup.age",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			if !encrypt {
				d, err := setupxfer.Build(p, version.Version, time.Now())
				if err != nil {
					return err
				}
				body, err := d.Marshal()
				if err != nil {
					return err
				}
				if out == "" {
					_, err = c.OutOrStdout().Write(body)
					return err
				}
				if err := fs.WriteTextAtomic(out, string(body), 0o600); err != nil {
					return err
				}
				_, _ = fmt.Fprintf(c.OutOrStdout(), "wrote %s: %d profile(s), %d key(s) named (none carried), %d token variable(s) named (no values)\n",
					out, len(d.Profiles), len(d.Keys), len(d.EnvNames))
				return nil
			}
			if out == "" {
				return errors.New("--encrypt writes a file: name it with --out (e.g. --out setup.age)")
			}
			if err := requireOwner(c, p, "exporting private keys and tokens"); err != nil {
				return err
			}
			pass, err := setupPassphrase(c, passFile, true)
			if err != nil {
				return err
			}
			// One consistent state, not a manifest another command is halfway
			// through writing; the lock also records the export in the history.
			lockForMutation(p)
			res, err := setupxfer.WriteFull(p, out, pass, version.Version, time.Now(), os.LookupEnv)
			if err != nil {
				return err
			}
			keys := 0
			for _, m := range res.Contents {
				if strings.HasPrefix(m, "ssh/") && !strings.HasSuffix(m, ".pub") {
					keys++
				}
			}
			log.Audit(p.AuditLog(), "setup.export", log.Field{Key: "encrypted", Value: true},
				log.Field{Key: "file", Value: res.Path}, log.Field{Key: "members", Value: len(res.Contents)})
			w := c.OutOrStdout()
			_, _ = fmt.Fprintf(w, "wrote %s (%s)\n  %d file(s), %d of them private keys or other key files; token values for the variables the setup names\n",
				res.Path, res.SHA256, len(res.Contents), keys)
			if len(res.Missing) > 0 {
				_, _ = fmt.Fprintf(w, "  no value here for %s, so the export carries none for it\n", strings.Join(res.Missing, ", "))
			}
			_, _ = fmt.Fprintf(w, "Keep the passphrase apart from the file. Import it with: %s setup import %s\n", brand.Command, res.Path)
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "out", "o", "", "write to this file (mode 0600) instead of stdout; required with --encrypt, and never replaced")
	cmd.Flags().BoolVar(&encrypt, "encrypt", false, "a full export: private keys, inventory and token values too, encrypted with a passphrase")
	cmd.Flags().StringVar(&passFile, "passphrase-file", "", "read the passphrase from this owner-only file instead of asking")
	return cmd
}

// setupPassphrase reads the passphrase from file, or asks for it at the
// terminal - twice when it is chosen, for an export.
func setupPassphrase(c *cobra.Command, file string, choose bool) ([]byte, error) {
	if file != "" {
		fi, err := os.Stat(file)
		if err != nil {
			return nil, err
		}
		if !platform.IsWindows() && fi.Mode().Perm()&0o077 != 0 {
			return nil, fmt.Errorf("%s is readable by other users (mode %04o); a passphrase file must be owner-only: chmod 600 %s",
				file, fi.Mode().Perm(), file)
		}
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		pass := strings.TrimRight(string(b), "\r\n")
		if pass == "" {
			return nil, fmt.Errorf("%s is empty", file)
		}
		return []byte(pass), nil
	}
	if !platform.StdinIsTerminal() {
		return nil, errors.New("no terminal to ask for the passphrase: pass --passphrase-file")
	}
	prompt := "passphrase for the export"
	if choose {
		_, _ = fmt.Fprintf(c.ErrOrStderr(), "Choose a passphrase of at least %d characters; it is the only way to open the file.\n", agepass.MinPassphrase)
	} else {
		prompt = "passphrase the export was made with"
	}
	first, err := platform.ReadSecret(prompt + ": ")
	if err != nil {
		return nil, err
	}
	if choose {
		again, err := platform.ReadSecret("again: ")
		if err != nil {
			return nil, err
		}
		if first != again {
			return nil, errors.New("the passphrases did not match")
		}
	}
	return []byte(first), nil
}

func newSetupImportCmd() *cobra.Command {
	var onConflict, passFile string
	var dryRun, yes, allowRisky bool
	cmd := &cobra.Command{
		Use:   "import FILE",
		Short: "Merge a setup document or an encrypted full export into this home, after showing what it would do",
		Example: "  " + brand.Command + " setup import setup.json --dry-run\n" +
			"  " + brand.Command + " setup import setup.age --on-conflict replace\n" +
			"  " + brand.Command + " setup import setup.age --on-conflict fail --passphrase-file ~/.setup-pass",
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			p, err := resolvePaths(c)
			if err != nil {
				return err
			}
			policy, err := setupxfer.ParsePolicy(onConflict)
			if err != nil {
				return err
			}
			head, err := fileHead(args[0])
			if err != nil {
				return err
			}
			if agepass.IsAge(head) {
				return importFull(c, p, args[0], passFile, policy, dryRun, yes, allowRisky)
			}
			body, err := os.ReadFile(args[0])
			if err != nil {
				return err
			}
			d, err := setupxfer.Parse(body)
			if err != nil {
				return err
			}
			if risky := d.Risky(); len(risky) > 0 && !allowRisky {
				return bundler.RiskyError(risky)
			}
			ch := change.SetupImport(d, nil, policy)
			plan, err := change.Preview(p, ch)
			if err != nil {
				return err
			}
			w := c.OutOrStdout()
			printSetupPlan(w, plan)
			if plan.Err != "" {
				return errors.New(plan.Err)
			}
			if dryRun || len(plan.Files) == 0 {
				if len(plan.Files) == 0 {
					_, _ = fmt.Fprintln(w, "nothing to change")
				}
				return nil
			}
			if err := confirmChange(c, "", yes); err != nil {
				return err
			}
			lockForMutation(p)
			snapshotBeforeMutation(p)
			return ch.Run(p, w)
		},
	}
	cmd.Flags().StringVar(&onConflict, "on-conflict", "keep", "what differs from this home: keep (leave it), replace (take the import's, keeping a copy), fail (write nothing)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what the import would add, keep and replace; write nothing")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "import without confirming (implied when there is no terminal)")
	cmd.Flags().StringVar(&passFile, "passphrase-file", "", "an encrypted export: read its passphrase from this owner-only file instead of asking")
	cmd.Flags().BoolVar(&allowRisky, "allow-risky-options", false, "import a manifest that sets SSH options that could redirect or weaken connections (you made it yourself)")
	return cmd
}

func printSetupPlan(w io.Writer, plan change.Plan) {
	if plan.Output != "" {
		_, _ = fmt.Fprint(w, plan.Output)
	}
	for _, f := range plan.Files {
		_, _ = fmt.Fprintf(w, "--- %s (%s)\n", f.Path, f.State)
		_, _ = fmt.Fprint(w, history.FormatDiff(f.Lines))
	}
}

// importFull imports an encrypted full export: the owner PIN, the
// passphrase, then a plan of every key file, config change and token name -
// never a value - before anything is written.
func importFull(c *cobra.Command, p paths.Paths, file, passFile string, policy setupxfer.Policy, dryRun, yes, allowRisky bool) error {
	if !agepassIsPassphraseFile(file) {
		return fmt.Errorf("%s is an age file encrypted to a key, not a passphrase: it is a backup bundle; restore it with `%s ssh restore`", file, brand.Command)
	}
	if err := requireOwner(c, p, "importing private keys and tokens"); err != nil {
		return err
	}
	pass, err := setupPassphrase(c, passFile, false)
	if err != nil {
		return err
	}
	replaced := bundler.ReplacedDir(p, time.Now())
	full, err := setupxfer.OpenFull(p, file, pass, replaced, allowRisky)
	if err != nil {
		return err
	}
	if risky := full.Risky(); len(risky) > 0 && !allowRisky {
		return bundler.RiskyError(risky)
	}
	keys, err := full.KeyItems(policy)
	if err != nil {
		return err
	}
	env, err := setupxfer.PlanEnv(p.EnvFile(), full.Env, policy)
	if err != nil {
		return err
	}
	ch := change.SetupImport(full.Doc, full.Inventory, policy)
	plan, err := change.Preview(p, ch)
	if err != nil {
		return err
	}
	w := c.OutOrStdout()
	_, _ = fmt.Fprintln(w, "key files (private keys are never shown):")
	_, _ = fmt.Fprint(w, keys.Format())
	_, _ = fmt.Fprintln(w, "token values in .env (names only):")
	_, _ = fmt.Fprint(w, env.Format())
	_, _ = fmt.Fprintln(w, "config:")
	printSetupPlan(w, plan)
	if plan.Err != "" {
		return errors.New(plan.Err)
	}
	if policy == setupxfer.Fail {
		var all []setupxfer.Item
		all = append(append(all, keys.Conflicts()...), env.Conflicts()...)
		if len(all) > 0 {
			return setupxfer.ConflictError{Items: all}
		}
	}
	kc, ec := keys.Counts(), env.Counts()
	if kc[setupxfer.StateAdd]+kc[setupxfer.StateReplace]+ec[setupxfer.StateAdd]+ec[setupxfer.StateReplace] == 0 && len(plan.Files) == 0 {
		_, _ = fmt.Fprintln(w, "nothing to change")
		return nil
	}
	if dryRun {
		return nil
	}
	if err := confirmChange(c, "", yes); err != nil {
		return err
	}
	lockForMutation(p)
	snapshotBeforeMutation(p)
	res, err := full.LayKeys(policy, keystore.New().Fingerprint)
	if err != nil {
		return err
	}
	if err := ch.Run(p, w); err != nil {
		return err
	}
	if _, err := setupxfer.MergeEnv(p.EnvFile(), full.Env, replaced, policy); err != nil {
		return err
	}
	finishErr := bundler.FinishRestore(p)
	log.Audit(p.AuditLog(), "setup.import", log.Field{Key: "encrypted", Value: true},
		log.Field{Key: "keys", Value: len(res.Restored)}, log.Field{Key: "tokens", Value: ec[setupxfer.StateAdd] + ec[setupxfer.StateReplace]})
	_, _ = fmt.Fprintf(w, "laid down %d key file(s); %d token value(s) written to %s\n", len(res.Restored),
		ec[setupxfer.StateAdd]+ec[setupxfer.StateReplace], p.EnvFile())
	if len(res.Replaced) > 0 {
		_, _ = fmt.Fprintf(w, "the files it replaced are kept in %s\n", res.ReplacedDir)
	}
	if finishErr != nil {
		return fmt.Errorf("imported, but ~/.ssh is not usable yet: %w", finishErr)
	}
	return nil
}

// fileHead reads the first bytes of file, enough to tell what it is.
func fileHead(file string) ([]byte, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, 64)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return head[:n], nil
}

// agepassIsPassphraseFile reports whether file is an age file encrypted with
// a passphrase: a full setup export rather than a backup bundle.
func agepassIsPassphraseFile(file string) bool {
	head, err := fileHead(file)
	return err == nil && agepass.IsPassphrase(head)
}
