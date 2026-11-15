package repair

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/sangehq/authama/src/internal/services/owner"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// ResetPlan is what a factory reset would do, for the person to read first.
type ResetPlan struct {
	Moves   [][2]string // from, to
	Unbinds bool        // the home is bound to an owner, and is unbound first
}

// PlanFactoryReset says what FactoryReset would move, and where.
func PlanFactoryReset(p paths.Paths, now time.Time) ResetPlan {
	stamp := now.UTC().Format("20060102-150405")
	var plan ResetPlan
	if c, _ := owner.Load(p); c != nil {
		plan.Unbinds = true
	}
	if fileExists(p.ConfigDir) {
		plan.Moves = append(plan.Moves, [2]string{p.ConfigDir, p.ConfigDir + ".reset-" + stamp})
	}
	if paths.SeparateState(p) && fileExists(p.State()) {
		plan.Moves = append(plan.Moves, [2]string{p.State(), p.State() + ".reset-" + stamp})
	}
	return plan
}

// FactoryReset puts authama back to how it was before it was set up, and
// deletes nothing: the config home (and the state home, when separate) are
// renamed aside, so renaming them back undoes it. ~/.ssh is not touched - the
// keys, the config file and known_hosts stay as they are.
//
// A bound home is unbound first. Its sealing key lives in the OS store, not in
// the folder that is moved, so a home renamed back later would otherwise come
// back bound to a key that a new binding may have replaced: asking for a PIN
// that no reset could replace. Unbinding first means a restored home simply
// has no owner, and can be bound again.
func FactoryReset(p paths.Paths, now time.Time) (ResetPlan, error) {
	plan := PlanFactoryReset(p, now)
	if len(plan.Moves) == 0 {
		return plan, errors.New("there is nothing to reset: no config home exists")
	}
	if plan.Unbinds {
		if err := owner.Forget(p); err != nil && !errors.Is(err, owner.ErrNotSetUp) {
			return plan, fmt.Errorf("unbinding the owner first: %w", err)
		}
	}
	for _, mv := range plan.Moves {
		if fileExists(mv[1]) {
			return plan, fmt.Errorf("%s already exists; nothing was moved over it", mv[1])
		}
		if err := os.Rename(mv[0], mv[1]); err != nil {
			return plan, fmt.Errorf("moving %s aside: %w", mv[0], err)
		}
	}
	return plan, nil
}
