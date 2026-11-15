package reconciler

import (
	"fmt"
	"io"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// ReplaceLocally is a forced rotation's fallback, as `ssh keygen --force`
// is for one key: each key named (profile/key) is minted afresh on this
// computer, its current key moved to old/, with nothing deployed. It is for a
// key that cannot be rotated in place - never deployed, or its host cannot be
// reached - and the new key has to be deployed afterwards. The manifest and
// inventory are read afresh, as a rotation before it may have saved them.
// replaced is what was minted; failed what could not be, each said in out.
func ReplaceLocally(p paths.Paths, names []string, emitUseKeychain bool, out io.Writer) (replaced, failed []string) {
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		_, _ = fmt.Fprintf(out, "cannot replace keys: %v\n", err)
		return nil, names
	}
	inv, err := inventory.Load(p.Inventory())
	if err != nil {
		_, _ = fmt.Fprintf(out, "cannot replace keys: %v\n", err)
		return nil, names
	}
	r := New(p, m, inv, emitUseKeychain)
	for _, n := range names {
		ref, err := m.ResolveKeySelector(n)
		if err != nil {
			failed = append(failed, n)
			_, _ = fmt.Fprintf(out, "FAIL  %s\n  FAIL  %v\n", n, err)
			continue
		}
		if _, err := r.Mint(ref.String(), "", map[manifest.KeyRef]bool{ref: true}); err != nil {
			failed = append(failed, n)
			_, _ = fmt.Fprintf(out, "FAIL  %s\n  FAIL  not replaced: %v\n", n, err)
			continue
		}
		replaced = append(replaced, n)
		_, _ = fmt.Fprintf(out, "ok    %s\n  next  replaced on this computer (the old key is in old/): deploy it next, from the key's page or `ssh deploy %s`\n", n, n)
	}
	return replaced, failed
}
