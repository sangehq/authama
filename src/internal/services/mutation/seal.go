package mutation

import (
	"fmt"
	"os"
	"time"

	"github.com/sangehq/authama/src/internal/brand"
	"github.com/sangehq/authama/src/internal/services/owner"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// sealState is what the owner seal said when a scope first took the lock.
type sealState struct {
	checked bool
	clean   bool
	p       paths.Paths
}

// noteSeal checks the owner seal the first time a scope takes the lock: before
// the scope writes anything, so what it finds is somebody else's doing. The
// caller holds recMu.
func noteSeal(f *frame, p paths.Paths) {
	if f.seal.checked {
		return
	}
	st := owner.Verify(p)
	f.seal = sealState{checked: true, clean: st.Clean(), p: p}
	if !f.seal.clean {
		_, _ = fmt.Fprintf(os.Stderr, brand.Command+": WARNING: %s. authama will not reseal over it; "+
			"`authama owner verify` shows what changed, and `authama owner reseal` accepts it.\n", st.Describe())
	}
}

// resealAfter remakes the seal once a scope that started clean has made its
// changes, so the files authama itself wrote are the sealed state. A scope
// that found the home already changed leaves the seal as it was: resealing
// then would bless the outside change along with authama's own.
func resealAfter(f *frame) {
	if !f.seal.checked || !f.seal.clean || f.scope.DryRun {
		return
	}
	if err := owner.Reseal(f.seal.p, time.Now()); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, brand.Command+": WARNING: the owner seal was not remade (%v); "+
			"the next run will report this operation's changes as made outside authama\n", err)
	}
}
