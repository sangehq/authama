package rotator

import (
	"fmt"
	"io"
	"strings"
)

// RotateEach rotates the keys named, one after another, as `authama ssh rotate`
// does for a selection: a key that fails is reported and left as it was, and
// the rest go on. save runs after every committed key, so an interrupted batch
// keeps what it finished. next, when set, runs before each key, and an error
// from it stops the batch there: a web job's cancel. It returns the keys not
// rotated.
func (r *Rotator) RotateEach(names []string, allowUnverified bool, pw string, out io.Writer,
	save, next func() error) ([]string, error) {
	var failed []string
	r.results = nil
	for _, n := range names {
		if next != nil {
			if err := next(); err != nil {
				return failed, err
			}
		}
		report, err := r.Rotate(n, allowUnverified, pw)
		res := RunResult{Ref: n, Err: err, Via: r.via(n)}
		if err == nil {
			res.Report = &report
		}
		r.results = append(r.results, res)
		switch {
		case err != nil:
			failed = append(failed, n)
			_, _ = fmt.Fprintf(out, "rotation FAILED: %s\n  %v\n", n, err)
			continue
		case report.Committed:
			if err := save(); err != nil {
				return failed, err
			}
		default:
			failed = append(failed, n)
		}
		_, _ = fmt.Fprintln(out, report.Format())
	}
	_, _ = fmt.Fprintf(out, "%d of %d key(s) rotated", len(names)-len(failed), len(names))
	if len(failed) > 0 {
		_, _ = fmt.Fprintf(out, "; not rotated (left as they were): %s\n", strings.Join(failed, ", "))
		return failed, nil
	}
	_, _ = fmt.Fprintln(out)
	return nil, nil
}

// Results is what the last RotateEach did with each key, for its summary.
func (r *Rotator) Results() []RunResult { return r.results }

// Via is how the key named reaches its hosts, for a summary of one key.
func (r *Rotator) Via(name string) string { return r.via(name) }

// via is how the key named reaches its hosts, as a summary's VIA column
// says it: each distinct way, comma-separated.
func (r *Rotator) via(name string) string {
	_, _, hosts, err := r.profileAndHosts(name)
	if err != nil {
		return ""
	}
	var vias []string
	for _, h := range hosts {
		vias = appendNew(vias, via(r.provider(h)))
	}
	return strings.Join(vias, ", ")
}
