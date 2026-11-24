package change

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/sangehq/authama/src/internal/core/manifest"
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
	rules, err := structure.LoadRules(p)
	if err != nil {
		return nil, err
	}
	auto := map[string]bool{}
	for _, r := range rules {
		if r.AutoApply {
			auto[r.ID] = true
		}
	}
	if len(auto) == 0 {
		return nil, nil
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
	if len(moves) == 0 {
		return nil, nil
	}
	mutation.SnapshotBeforeMutation(p, 10)
	if err := StructurePlan(moves).Run(p, out); err != nil {
		return nil, err
	}
	return ids, nil
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
