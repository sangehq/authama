package web

import "github.com/simtabi/authama/src/internal/services/tokensvc"

// auditRow is one tool's line in the token audit card: its file, its mode,
// and a note that belongs with it (a present auth.json overrides
// $COMPOSER_AUTH).
type auditRow struct {
	Target, Path, Mode, Note string
}

// auditProblem is one thing the token audit wants fixed: a credential file
// others can read, or a .gitignore that does not cover the secret files.
type auditProblem struct {
	Target, Path, Mode string
	Missing            []string // set for a .gitignore
}

// tokenAudit is the token audit laid out for the page: what to fix first,
// then the files that passed, then the tools with no file here, which are
// folded away.
type tokenAudit struct {
	Problems []auditProblem
	Passed   []auditRow
	Absent   []auditRow
}

// layOutAudit groups the audit's findings for the card. A file others can read
// is one problem, though the audit says so in two lines.
func layOutAudit(fs []tokensvc.Finding) tokenAudit {
	var a tokenAudit
	notes := map[string]string{}
	for _, f := range fs {
		if f.Level == "note" {
			notes[f.Target] = f.Text
		}
	}
	seen := map[string]bool{}
	for _, f := range fs {
		switch f.Level {
		case "warn":
			key := f.Target + "|" + f.Path
			if seen[key] {
				continue
			}
			seen[key] = true
			a.Problems = append(a.Problems, auditProblem{Target: f.Target, Path: f.Path, Mode: f.Mode, Missing: f.Missing})
		case "ok":
			a.Passed = append(a.Passed, auditRow{Target: f.Target, Path: f.Path, Mode: f.Mode, Note: composerNote(notes[f.Target])})
		case "absent":
			a.Absent = append(a.Absent, auditRow{Target: f.Target, Path: f.Path})
		}
	}
	return a
}

// composerNote is the audit's note about a tool, in the page's own voice.
func composerNote(text string) string {
	if text == "" {
		return ""
	}
	return "While this file exists, Composer reads it instead of $COMPOSER_AUTH."
}
