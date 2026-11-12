package web

import (
	"regexp"
	"strings"

	"github.com/simtabi/authama/src/internal/services/jobs"
)

// consoleLine is one line of a job's output, read for the page: how deep it
// is indented, the status word it starts with and how that reads (ok, bad,
// warn, info, muted), the host or key it is about, and the rest. The CLI
// prints plain text; the page colours and aligns it from these.
type consoleLine struct {
	// Last marks the last line under its parent, drawn └─ rather than ├─.
	Last            bool
	Indent          int
	Tag, Kind       string
	Name, Text, Raw string
}

// consoleTags are the status words jobs start a line with, and how each reads.
var consoleTags = map[string]string{
	"ok": "ok", "done": "ok", "revoked": "ok", "deleted": "ok", "pinned": "ok", "deployed": "ok",
	"FAIL": "bad", "FAILED": "bad", "error:": "bad",
	"paste": "warn", "WARNING:": "warn", "warning:": "warn",
	"skip": "muted", "rotate": "info", "move": "info", "next": "info",
}

// consoleName is a line's subject: a host alias or profile/key, then a colon.
var consoleName = regexp.MustCompile(`^([A-Za-z0-9_.@/-]+):\s+(.*)$`)

// readConsoleLine reads one line of output. The same rules are in the page's
// script (console.js), for the lines that arrive while the job runs.
func readConsoleLine(raw string) consoleLine {
	l := consoleLine{Raw: raw}
	trimmed := strings.TrimLeft(raw, " ")
	l.Indent = (len(raw) - len(trimmed) + 1) / 2
	rest := trimmed
	if word, after, ok := strings.Cut(trimmed, " "); ok || trimmed != "" {
		if kind, known := consoleTags[word]; known {
			l.Tag, l.Kind, rest = strings.TrimSuffix(word, ":"), kind, strings.TrimSpace(after)
		}
	}
	switch {
	case l.Kind != "":
	case strings.Contains(rest, "FAILED") || strings.Contains(rest, "NOT ") || strings.HasPrefix(rest, "cannot "):
		l.Kind = "bad"
	case strings.Contains(rest, "WARNING"):
		l.Kind = "warn"
	}
	if m := consoleName.FindStringSubmatch(rest); m != nil {
		l.Name, l.Text = m[1], m[2]
	} else if l.Tag != "" && !strings.Contains(rest, " ") {
		l.Name = rest
	} else {
		l.Text = rest
	}
	return l
}

// consoleLines reads every line of a job's output for the page.
func consoleLines(lines []string) []consoleLine {
	out := make([]consoleLine, 0, len(lines))
	for _, ln := range lines {
		out = append(out, readConsoleLine(ln))
	}
	// A nested line is the last of its group when no line at its depth
	// follows before a shallower one.
	for i := range out {
		if out[i].Indent == 0 {
			continue
		}
		out[i].Last = i+1 == len(out) || out[i+1].Indent < out[i].Indent
	}
	return out
}

// tally is how a job's items ended: passed (done), failed, and still to go.
type tally struct{ Done, Failed, Waiting int }

// itemTally counts a job's items by state; nil when it planned none.
func itemTally(items []jobs.Item) *tally {
	if len(items) == 0 {
		return nil
	}
	var t tally
	for _, it := range items {
		switch it.State {
		case jobs.ItemDone:
			t.Done++
		case jobs.ItemFailed:
			t.Failed++
		default:
			t.Waiting++
		}
	}
	return &t
}
