package change

import (
	"io"
	"os"

	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/services/structure"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// StructurePlan is `authama ssh structure approve`: an approved plan of key
// moves (see services/structure), never a suggestion carried out unasked.
// Moves within ~/.ssh happen in the preview's copy too - a private key's
// placeholder is renamed - so the preview shows each as a file removed and
// one created, beside the manifest and config diffs. A key taken from the
// config home is moved on apply only (the copy does not hold it), which the
// effects say.
func StructurePlan(moves []structure.Move) Change {
	args := make([]string, 0, len(moves))
	var subjects []history.Subject
	seen := map[string]bool{}
	for _, mv := range moves {
		args = append(args, mv.Line())
		if !seen[mv.Profile] {
			seen[mv.Profile] = true
			subjects = append(subjects, history.Subject{Kind: history.KindProfile, Name: mv.Profile})
		}
	}
	return Change{
		Command: "ssh structure approve", Args: args, Subjects: subjects,
		Effects: func(p paths.Paths) ([]string, error) {
			out := structure.ConfigHomeEffects(p, moves)
			return append(out, "copies each key it moves (owner-only) to .state/moved/<time>/ in the state home "+
				"first, and records each move in history"), nil
		},
		OnApply: func(p paths.Paths, out io.Writer) error {
			return structure.MoveConfigHome(p, moves, out)
		},
		Run: func(p paths.Paths, out io.Writer) error {
			if _, err := structure.Apply(p, moves, out); err != nil {
				return err
			}
			if _, err := os.Stat(p.Manifest()); err == nil {
				return ApplyManifestEdit(out, p)
			}
			return nil
		},
	}
}
