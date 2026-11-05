package change

import (
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/simtabi/authama/src/internal/services/mutation"
	"github.com/simtabi/authama/src/internal/services/repair"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// Repair is `authama repair --yes`: every fix repair.Home.Check finds. The
// fixes reach the OS key store, file modes and a database, which a preview
// copy does not hold, so they run in OnApply; Effects lists them, and the
// confirmation is bound to that list.
func Repair(home func(paths.Paths) repair.Home) Change {
	fixable := func(p paths.Paths) []repair.Finding {
		var out []repair.Finding
		for _, f := range home(p).Check() {
			if f.Fixable {
				out = append(out, f)
			}
		}
		return out
	}
	return Change{
		Command: "repair", Args: []string{"--yes"},
		Effects: func(p paths.Paths) ([]string, error) {
			fs := fixable(p)
			if len(fs) == 0 {
				return nil, errors.New("nothing here can be fixed automatically")
			}
			var lines []string
			for _, f := range fs {
				lines = append(lines, fmt.Sprintf("[%s] %s: %s", f.Area, f.Problem, f.Fix))
			}
			return lines, nil
		},
		OnApply: func(p paths.Paths, out io.Writer) error { return repair.Fix(fixable(p), out) },
		Run:     func(paths.Paths, io.Writer) error { return nil },
	}
}

// FactoryReset is `authama reset --confirm reset`. confirmed is the word the
// form asks to be typed.
func FactoryReset(confirmed bool) Change {
	return Change{
		Command: "reset", Args: []string{"--confirm", "reset"}, NoSnapshot: true,
		Effects: func(p paths.Paths) ([]string, error) {
			if !confirmed {
				return nil, errors.New(`type "reset" to confirm`)
			}
			plan := repair.PlanFactoryReset(p, time.Now())
			if len(plan.Moves) == 0 {
				return nil, errors.New("there is nothing to reset: no config home exists")
			}
			var lines []string
			if plan.Unbinds {
				lines = append(lines, "unbinds the owner first: the PIN and the sealing key go")
			}
			for _, mv := range plan.Moves {
				lines = append(lines, fmt.Sprintf("moves %s aside, kept, to %s.reset-<time>", mv[0], mv[0]))
			}
			return append(lines, "does not touch ~/.ssh: keys, config and known_hosts stay",
				"this page then shows an empty home; `authama init` starts again"), nil
		},
		OnApply: func(p paths.Paths, out io.Writer) error {
			// The lock's file is inside the state home being moved; Windows
			// will not rename a folder holding an open file.
			mutation.FinishEarly()
			mutation.Release()
			plan, err := repair.FactoryReset(p, time.Now())
			for _, mv := range plan.Moves {
				_, _ = fmt.Fprintf(out, "moved %s -> %s\n", mv[0], mv[1])
			}
			return err
		},
		Run: func(paths.Paths, io.Writer) error { return nil },
	}
}
