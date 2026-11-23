package change

import (
	"errors"
	"fmt"
	"io"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/services/bundler"
	"github.com/sangehq/authama/src/internal/services/keystore"
	"github.com/sangehq/authama/src/internal/services/setupxfer"
	"github.com/sangehq/authama/src/internal/util/log"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// SetupImportFull is an encrypted full export, already opened, merged under
// policy: what `setup import FILE.age` does. Its preview lists every key
// file and token by name (private keys and token values are never shown),
// and the config diffs as a setup document's import shows them. The key
// files and token values are laid down only on apply; the files they replace
// are kept in replaced. A full export with risky content (setupxfer.Doc.Risky) is refused
// unless allowRisky.
func SetupImportFull(full *setupxfer.Full, policy setupxfer.Policy, replaced string, allowRisky bool) (Change, error) {
	if risky := full.Risky(); len(risky) > 0 && !allowRisky {
		return Change{}, setupxfer.RiskyError(risky, brand.Command)
	}
	ch := SetupImport(full.Doc, full.Inventory, policy)
	ch.Args = append([]string{"FILE.age"}, ch.Args...)
	ch.Effects = func(p paths.Paths) ([]string, error) {
		keys, err := full.KeyItems(policy)
		if err != nil {
			return nil, err
		}
		env, err := setupxfer.PlanEnv(p.EnvFile(), full.Env, policy)
		if err != nil {
			return nil, err
		}
		if policy == setupxfer.Fail {
			var all []setupxfer.Item
			all = append(append(all, keys.Conflicts()...), env.Conflicts()...)
			if len(all) > 0 {
				return nil, setupxfer.ConflictError{Items: all}
			}
		}
		var out []string
		for _, it := range keys.Items {
			out = append(out, "key file "+it.Name+": "+it.State)
		}
		for _, it := range env.Items {
			out = append(out, "token value "+it.Name+" in .env: "+it.State)
		}
		for _, n := range full.EnvSkipped {
			out = append(out, "token value "+n+" in .env: not imported (the setup reads no token from it, or it would change how programs run)")
		}
		if len(out) == 0 {
			out = append(out, "no key file or token value to lay down")
		}
		return out, nil
	}
	ch.OnApply = func(p paths.Paths, out io.Writer) error {
		res, err := full.LayKeys(policy, keystore.New().Fingerprint)
		if err != nil {
			return err
		}
		env, err := setupxfer.MergeEnv(p.EnvFile(), full.Env, replaced, policy)
		if err != nil {
			return err
		}
		c := env.Counts()
		_, _ = fmt.Fprintf(out, "laid down %d key file(s); %d token value(s) written to %s\n", len(res.Restored),
			c[setupxfer.StateAdd]+c[setupxfer.StateReplace], p.EnvFile())
		if len(res.Replaced) > 0 {
			_, _ = fmt.Fprintf(out, "the files it replaced are kept in %s\n", res.ReplacedDir)
		}
		log.Audit(p.AuditLog(), "setup.import", log.Field{Key: "encrypted", Value: true},
			log.Field{Key: "keys", Value: len(res.Restored)}, log.Field{Key: "tokens", Value: c[setupxfer.StateAdd] + c[setupxfer.StateReplace]})
		return nil
	}
	run := ch.Run
	ch.Run = func(p paths.Paths, out io.Writer) error {
		if err := run(p, out); err != nil {
			return err
		}
		if err := bundler.FinishRestore(p); err != nil {
			return errors.Join(errors.New("imported, but ~/.ssh is not usable yet"), err)
		}
		return nil
	}
	return ch, nil
}
