package change

import (
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/services/mutation"
	"github.com/sangehq/authama/src/internal/services/structure"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// AutoApplyRuled carries out purpose-key suggestions that match a rule the
// owner enabled with --auto: confidence 1.0, a complete plan, a snapshot and
// a history entry. Copy-remove (deleting a leftover) is never auto-applied.
// A git-signing move is skipped unless sange is on PATH, so the signingkey
// handoff can run.
func AutoApplyRuled(p paths.Paths, ss []structure.Suggestion, out io.Writer) (applied []string, err error) {
	moves, ids, err := ruledMoves(p, ss, out)
	if err != nil || len(moves) == 0 {
		return nil, err
	}
	return ids, applyRuledMoves(p, moves, out)
}

// ruledMoves picks the moves AutoApplyRuled would make, and the suggestions
// they come from, without changing anything.
func ruledMoves(p paths.Paths, ss []structure.Suggestion, out io.Writer) ([]structure.Move, []string, error) {
	rules, err := structure.LoadRules(p)
	if err != nil {
		return nil, nil, err
	}
	auto := map[string]bool{}
	for _, r := range rules {
		if r.AutoApply {
			auto[r.ID] = true
		}
	}
	if len(auto) == 0 {
		return nil, nil, nil
	}
	var moves []structure.Move
	var ids []string
	for _, s := range ss {
		if s.Dismissed || s.Kind != structure.KindPurpose || s.Confidence < 1.0 || strings.TrimSpace(s.Plan) == "" {
			continue
		}
		if !ruleAutoMatches(s, auto) {
			continue
		}
		if s.Tool == manifest.ToolGit {
			if _, err := exec.LookPath("sange"); err != nil {
				_, _ = fmt.Fprintf(out, "skip auto-apply of %s: sange is not on PATH (git signing is updated through sange)\n", s.ID)
				continue
			}
		}
		mv, err := structure.ParsePlan(s.Plan)
		if err != nil || len(mv) == 0 {
			continue
		}
		moves = append(moves, mv...)
		ids = append(ids, s.ID)
	}
	return moves, ids, nil
}

// applyRuledMoves snapshots ~/.ssh and makes moves.
func applyRuledMoves(p paths.Paths, moves []structure.Move, out io.Writer) error {
	mutation.SnapshotBeforeMutation(p, 10)
	return StructurePlan(moves).Run(p, out)
}

func ruleAutoMatches(s structure.Suggestion, auto map[string]bool) bool {
	for _, ev := range s.Evidence {
		const pfx = "rule "
		if strings.HasPrefix(ev, pfx) {
			id := strings.TrimSpace(strings.TrimPrefix(ev, pfx))
			if auto[id] {
				return true
			}
			for aid := range auto {
				if len(id) >= 4 && strings.HasPrefix(aid, id) || len(aid) >= 4 && strings.HasPrefix(id, aid) {
					return true
				}
			}
		}
	}
	return false
}

// AutoApplyForServer is AutoApplyRuled for a process that does not exit, the
// web server's watcher. It waits its turn behind a web job or change like
// Apply does, and runs as one scoped operation, so the advisory lock is
// released when it returns: AutoApplyRuled alone holds it for the rest of the
// process, which a CLI command can do and `serve` cannot (audit F-18).
//
// It decides what to move first, with no lock: the watcher calls it after
// every change it sees and almost always nothing matches, and then it must
// neither queue behind a web job nor open an operation.
func AutoApplyForServer(p paths.Paths, ss []structure.Suggestion, out io.Writer, source string) ([]string, error) {
	moves, ids, err := ruledMoves(p, ss, out)
	if err != nil || len(moves) == 0 {
		return nil, err
	}
	release, err := mutation.Exclusive(busyWait)
	if err != nil {
		return nil, err
	}
	defer release()
	// It may have waited behind a web job that dismissed a suggestion or
	// turned a rule's --auto off, so what to move is decided again, from the
	// suggestions and rules as they are now, before anything moves.
	ids = nil
	scope := mutation.Scope{Command: "ssh structure check", Source: source, OpID: history.NewOpID()}
	err = mutation.Scoped(p, scope, func() error {
		fresh, _, err := structure.Check(p, time.Now())
		if err != nil {
			return err
		}
		var moves []structure.Move
		if moves, ids, err = ruledMoves(p, fresh, out); err != nil || len(moves) == 0 {
			return err
		}
		return applyRuledMoves(p, moves, out)
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}
