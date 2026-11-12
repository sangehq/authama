package rotator

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/simtabi/authama/src/internal/util/paths"
)

// The result words a summary uses: a rotation's, and a dry run's.
const (
	ResultRotated     = "rotated"
	ResultReplaced    = "replaced"
	ResultNotRotated  = "not rotated"
	ResultWouldRotate = "would rotate"
	ResultWouldPause  = "would pause"
	ResultWouldFail   = "would fail"
	// ResultWaiting is a key the run has not reached yet.
	ResultWaiting = "waiting"
	// KindReplaced is a key a forced rotation replaced on this computer.
	KindReplaced = "replaced"
	// KindNotRemoved is a key left trusted where it could not be removed.
	KindNotRemoved = "not-removed"
)

// SummaryKey is one key's line in a summary.
type SummaryKey struct {
	// Ref is profile/key.
	Ref string
	// Result is one of the Result words; Severity orders them, worst first:
	// 2 failed, 1 needs you, 0 went through.
	Result   string
	Severity int
	// Via is how the new key reaches its hosts (ssh, github api, ploi).
	Via string
	// Kind names why, as HostCheck.Kind does; Note is the long explanation,
	// printed once under Notes however many keys share it.
	Kind, Note string
	// Pending is a key the run has not finished yet: Result is ResultWaiting,
	// or what the run is doing to it now (testing, rotating, revoking).
	Pending bool
}

// Summary is the outcome of a rotation, or of its dry run, for every key it
// covered: what each key's result was, grouped by owner and project, with each
// explanation printed once as a numbered note, what is left for you to do, and
// the command that runs the failed keys again.
type Summary struct {
	Check   bool
	Keys    []SummaryKey
	Elapsed time.Duration
	// Command is the command line the run was, as its first line shows it:
	// "authama ssh rotate --all".
	Command string
	// Verb is the tree's root, the command's own name: rotate (the default),
	// revoke, delete. RerunCommand and UntouchedText replace the rotation's
	// own re-run command and closing note, for another command's summary.
	Verb, RerunCommand, UntouchedText string
}

// SummaryRow is one line of a summary's tree: an owner or a project group
// (Group, with its Counts), or a key.
type SummaryRow struct {
	// Branch is the tree drawn before the label (├─, │, └─). Root is the
	// tree's first line, the command's own name.
	Branch, Label string
	Root          bool
	// Env is a project group's environment, drawn after its name: simbi · prd.
	Env    string
	Group  bool
	Counts string
	// Mark is ✓, ! or × for a key.
	Mark, Key, Via, Result, Reason string
	Severity                       int
	// Note numbers the key's explanation under Notes; 0 for none.
	Note int
	// Pending is a key not finished yet: waiting, or under way.
	Pending bool
	// Gap puts a line before the row that holds only the trunk (Trunk):
	// between owners.
	Gap   bool
	Trunk string
}

// SummaryNote is one explanation, numbered, with the severity of the worst
// key it explains.
type SummaryNote struct {
	N        int
	Text     string
	Severity int
}

// SummaryNeed is one thing left for you to do, and the keys it is for.
type SummaryNeed struct {
	Mark, Action string
	Severity     int
	Note         int
	Keys         string
}

// SummarizeCheck is a dry run's summary: each key's worst host decides its
// result.
func SummarizeCheck(reports []CheckReport, elapsed time.Duration) Summary {
	s := Summary{Check: true, Elapsed: elapsed}
	for _, rep := range reports {
		k := SummaryKey{Ref: rep.Key, Result: ResultWouldRotate, Kind: KindLogsIn}
		var vias []string
		for _, h := range rep.Hosts {
			vias = appendNew(vias, h.Via)
			sev := 0
			switch {
			case !h.OK:
				sev = 2
			case h.Manual:
				sev = 1
			}
			if sev > k.Severity || (k.Note == "" && sev == k.Severity && h.Kind != KindLogsIn) {
				k.Severity, k.Kind = sev, h.Kind
				if h.Kind != KindLogsIn && h.Kind != KindAPI {
					k.Note = h.Message
				}
			}
		}
		if rep.Problem != "" {
			k.Severity, k.Kind, k.Note = 2, rep.ProblemKind, rep.Problem
		}
		k.Via = strings.Join(vias, ", ")
		k.Result = [...]string{ResultWouldRotate, ResultWouldPause, ResultWouldFail}[k.Severity]
		s.Keys = append(s.Keys, k)
	}
	return s
}

// RunResult is one key's outcome in a batch rotation.
type RunResult struct {
	Ref    string
	Report *RotateReport
	Err    error
	Via    string
}

// SummarizeRun is a rotation's summary. replaced names the keys a forced
// rotation then replaced on this computer.
func SummarizeRun(results []RunResult, replaced []string, elapsed time.Duration) Summary {
	s := Summary{Elapsed: elapsed}
	forced := map[string]bool{}
	for _, n := range replaced {
		forced[n] = true
	}
	for _, res := range results {
		k := SummaryKey{Ref: res.Ref, Via: res.Via, Result: ResultRotated, Kind: KindLogsIn}
		switch {
		case res.Err == nil && res.Report != nil && res.Report.Committed:
		case forced[res.Ref]:
			k.Result, k.Severity, k.Kind = ResultReplaced, 1, KindReplaced
			k.Note = "replaced on this computer, not deployed: the old key is in old/ and still the one its hosts trust; deploy the new key next, or roll it back"
		default:
			k.Result, k.Severity = ResultNotRotated, 2
			msg := ""
			if res.Err != nil {
				msg = res.Err.Error()
			} else if res.Report != nil {
				msg = res.Report.Message
			}
			k.Kind, k.Note = classify(msg), msg
		}
		s.Keys = append(s.Keys, k)
	}
	return s
}

// classify names why a rotation failed, from what it said.
func classify(msg string) string {
	m := strings.ToLower(msg)
	switch {
	case strings.Contains(m, "unreachable"), strings.Contains(m, "no route"), strings.Contains(m, "timed out"):
		return KindUnreachable
	case strings.Contains(m, "never deployed"):
		return KindNeverDeployed
	case strings.Contains(m, "not on disk"):
		return KindNoKey
	}
	return KindFailed
}

// reasons are a kind's short reason in the tree, and needs what it leaves
// you to do.
var (
	reasons = map[string]string{KindLogsIn: "logs in", KindManual: "awaiting paste", KindUnreachable: "host unreachable",
		KindNeverDeployed: "never deployed", KindRemoved: "key removed there", KindNoKey: "not on disk",
		KindFailed: "failed", KindReplaced: "deploy next", KindNotRemoved: "not removed there"}
	needs = map[string]string{KindUnreachable: "check network", KindNeverDeployed: "deploy first",
		KindRemoved: "deploy again", KindManual: "paste by hand", KindNoKey: "reconcile first",
		KindFailed: "see the note", KindReplaced: "deploy next", KindNotRemoved: "remove by hand"}
	marks = [...]string{"✓", "!", "×"}
)

// Totals are the counts of each result, worst first: "3 would fail · 3 would
// pause · 3 would rotate".
func (s Summary) Totals() string { return counts(s.Keys) }

// SummaryCount is how many keys had one result.
type SummaryCount struct {
	Word     string
	N        int
	Severity int
}

// Counts are the counts of each result, worst first.
func (s Summary) Counts() []SummaryCount { return tally(s.Keys) }

func tally(keys []SummaryKey) []SummaryCount {
	var all []SummaryCount
	at := map[string]int{}
	for _, k := range keys {
		i, ok := at[k.Result]
		if !ok {
			i = len(all)
			at[k.Result] = i
			all = append(all, SummaryCount{Word: k.Result, Severity: k.Severity})
		}
		all[i].N++
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Severity > all[j].Severity })
	return all
}

func counts(keys []SummaryKey) string {
	var parts []string
	for _, c := range tally(keys) {
		parts = append(parts, fmt.Sprintf("%d %s", c.N, c.Word))
	}
	return strings.Join(parts, " · ")
}

// Took is how long the run took, to a tenth of a second; "" when unknown.
func (s Summary) Took() string {
	d := s.Elapsed.Round(100 * time.Millisecond)
	if d <= 0 {
		return ""
	}
	return d.String()
}

// Owners is how many profiles the summary covers.
func (s Summary) Owners() int {
	seen := map[string]bool{}
	for _, k := range s.Keys {
		seen[profileOf(k.Ref)] = true
	}
	return len(seen)
}

// Failed is the keys that did not go through.
func (s Summary) Failed() []string {
	var out []string
	for _, k := range s.Keys {
		if k.Severity == 2 {
			out = append(out, k.Ref)
		}
	}
	return out
}

// Rerun is the command that runs the failed keys again, or "": rotate
// --failed reads the keys the last run left.
func (s Summary) Rerun() string {
	if len(s.Failed()) == 0 || s.Running() {
		return ""
	}
	if s.RerunCommand != "" {
		return s.RerunCommand
	}
	if s.Check {
		return "authama ssh rotate --failed --check"
	}
	return "authama ssh rotate --failed"
}

// Notes are the explanations, each once, numbered in the order the run met
// them.
func (s Summary) Notes() []SummaryNote {
	notes, _ := s.numbered()
	return notes
}

func (s Summary) numbered() ([]SummaryNote, map[string]int) {
	var notes []SummaryNote
	index := map[string]int{}
	for _, k := range s.Keys {
		if k.Note == "" {
			continue
		}
		if i := index[k.Note]; i > 0 {
			notes[i-1].Severity = max(notes[i-1].Severity, k.Severity)
			continue
		}
		notes = append(notes, SummaryNote{N: len(notes) + 1, Text: k.Note, Severity: k.Severity})
		index[k.Note] = len(notes)
	}
	return notes, index
}

// CountsUp are the counts of each result, best first, as the Output card's
// badges show them.
func (s Summary) CountsUp() []SummaryCount {
	c := tally(s.Keys)
	for i, j := 0, len(c)-1; i < j; i, j = i+1, j-1 {
		c[i], c[j] = c[j], c[i]
	}
	return c
}

// Root is the tree's first line: the command's own name, rotate unless Verb
// says otherwise.
func (s Summary) Root() string {
	if s.Verb != "" {
		return s.Verb
	}
	return "rotate"
}

// Running reports whether a key is not finished yet.
func (s Summary) Running() bool {
	for _, k := range s.Keys {
		if k.Pending {
			return true
		}
	}
	return false
}

// Pending is the keys a run has yet to finish, for a summary drawn while it
// runs: the first is under way (Result is now, "testing" say), the rest wait.
func Pending(names []string, now string) []SummaryKey {
	out := make([]SummaryKey, len(names))
	for i, n := range names {
		out[i] = SummaryKey{Ref: n, Result: ResultWaiting, Pending: true}
		if i == 0 && now != "" {
			out[i].Result = now
		}
	}
	return out
}

// leaf is a key under its owner, with its short name.
type leaf struct {
	key  SummaryKey
	name string
}

// item is a project group, or a single key (label "").
type item struct {
	label string
	keys  []leaf
}

// owner is a profile's keys.
type owner struct {
	name  string
	items []item
}

func severity(keys []leaf) int {
	worst := 0
	for _, k := range keys {
		worst = max(worst, k.key.Severity)
	}
	return worst
}

// envs are the words a key's name may carry as its environment.
var envs = map[string]bool{"prd": true, "prod": true, "production": true, "stg": true, "stage": true, "staging": true,
	"dev": true, "develop": true, "test": true, "qa": true, "uat": true, "sandbox": true, "preview": true, "demo": true}

// owners groups the keys by profile, then by project (and environment) read
// from each key's name, worst first at every level and otherwise in order.
func (s Summary) owners() []owner {
	var order []string
	byOwner := map[string][]SummaryKey{}
	for _, k := range s.Keys {
		p := profileOf(k.Ref)
		if _, ok := byOwner[p]; !ok {
			order = append(order, p)
		}
		byOwner[p] = append(byOwner[p], k)
	}
	var out []owner
	for _, p := range order {
		keys := byOwner[p]
		names := make([][]string, len(keys))
		first := map[string]int{}
		for i, k := range keys {
			names[i] = strings.Split(shortName(p, k.Ref), "-")
			first[names[i][0]]++
		}
		var items []item
		at := map[string]int{}
		for i, k := range keys {
			seg := names[i]
			label, rest := "", strings.Join(seg, "-")
			switch {
			case len(seg) >= 3 && envs[seg[1]]:
				label, rest = seg[0]+" · "+seg[1], strings.Join(seg[2:], "-")
			case len(seg) >= 2 && first[seg[0]] >= 2:
				label, rest = seg[0], strings.Join(seg[1:], "-")
			}
			if label == "" {
				items = append(items, item{keys: []leaf{{k, rest}}})
				continue
			}
			if j, ok := at[label]; ok {
				items[j].keys = append(items[j].keys, leaf{k, rest})
				continue
			}
			at[label] = len(items)
			items = append(items, item{label: label, keys: []leaf{{k, rest}}})
		}
		if !s.Running() {
			for _, it := range items {
				sort.SliceStable(it.keys, func(a, b int) bool { return it.keys[a].key.Severity > it.keys[b].key.Severity })
			}
			sort.SliceStable(items, func(a, b int) bool { return severity(items[a].keys) > severity(items[b].keys) })
		}
		out = append(out, owner{name: p, items: items})
	}
	// While the run is under way the tree keeps the run's order, so rows do
	// not jump as results come in; finished, it is worst first.
	if !s.Running() {
		sort.SliceStable(out, func(a, b int) bool { return ownerSeverity(out[a]) > ownerSeverity(out[b]) })
	}
	return out
}

func ownerSeverity(o owner) int {
	worst := 0
	for _, it := range o.items {
		worst = max(worst, severity(it.keys))
	}
	return worst
}

func profileOf(ref string) string {
	p, _, _ := strings.Cut(ref, "/")
	return p
}

// shortName is a key's name without its profile prefix and key type suffix:
// simtabi_simbi-prd-web-00-ed25519 reads simbi-prd-web-00.
func shortName(profile, ref string) string {
	_, name, _ := strings.Cut(ref, "/")
	name = strings.TrimPrefix(name, profile+"_")
	for _, t := range []string{"-ed25519-sk", "-ecdsa-sk", "-ed25519", "-ecdsa", "-rsa"} {
		if trimmed, ok := strings.CutSuffix(name, t); ok && trimmed != "" {
			return trimmed
		}
	}
	return name
}

// Rows is the summary's tree, line by line, under one continuous trunk:
// the root, each owner, its project groups and its keys.
func (s Summary) Rows() []SummaryRow {
	_, index := s.numbered()
	rows := []SummaryRow{{Label: s.Root(), Root: true, Group: true}}
	keyRow := func(branch string, l leaf) SummaryRow {
		mark := marks[l.key.Severity]
		switch {
		case l.key.Pending && l.key.Result == ResultWaiting:
			mark = "·"
		case l.key.Pending:
			mark = "›"
		}
		return SummaryRow{Branch: branch, Label: l.name, Mark: mark, Key: keyName(l.key.Ref), Pending: l.key.Pending,
			Via: l.key.Via, Result: l.key.Result, Reason: reasons[l.key.Kind], Severity: l.key.Severity, Note: index[l.key.Note]}
	}
	owners := s.owners()
	for oi, o := range owners {
		lastOwner := oi == len(owners)-1
		ob, trunk := "├─ ", "│  "
		if lastOwner {
			ob, trunk = "└─ ", "   "
		}
		var all []SummaryKey
		for _, it := range o.items {
			for _, l := range it.keys {
				all = append(all, l.key)
			}
		}
		rows = append(rows, SummaryRow{Branch: ob, Label: o.name, Group: true, Counts: counts(all), Severity: ownerSeverity(o),
			Gap: oi > 0, Trunk: "│"})
		for ii, it := range o.items {
			branch, under := "├─ ", "│  "
			if ii == len(o.items)-1 {
				branch, under = "└─ ", "   "
			}
			if it.label == "" {
				rows = append(rows, keyRow(trunk+branch, it.keys[0]))
				continue
			}
			var keys []SummaryKey
			for _, l := range it.keys {
				keys = append(keys, l.key)
			}
			name, env, _ := strings.Cut(it.label, " · ")
			rows = append(rows, SummaryRow{Branch: trunk + branch, Label: name, Env: env, Group: true, Counts: counts(keys), Severity: severity(it.keys)})
			for ki, l := range it.keys {
				b := "├─ "
				if ki == len(it.keys)-1 {
					b = "└─ "
				}
				rows = append(rows, keyRow(trunk+under+b, l))
			}
		}
	}
	return rows
}

// label is a row's name as the text output draws it: a group's environment
// after a middle dot.
func (r SummaryRow) label() string {
	if r.Env != "" {
		return r.Label + " · " + r.Env
	}
	return r.Label
}

// Needs is what is left for you to do, worst first, each with the keys it is
// for, grouped as the tree groups them.
func (s Summary) Needs() []SummaryNeed {
	_, index := s.numbered()
	type acc struct {
		need  SummaryNeed
		paths []string
		keys  map[string][]string
	}
	var order []*acc
	by := map[string]*acc{}
	for _, o := range s.owners() {
		for _, it := range o.items {
			path := o.name
			if it.label != "" {
				path += "/" + strings.ReplaceAll(it.label, " · ", "·")
			}
			for _, l := range it.keys {
				action := needs[l.key.Kind]
				if l.key.Severity == 0 || action == "" {
					continue
				}
				a := by[l.key.Kind]
				if a == nil {
					a = &acc{need: SummaryNeed{Mark: marks[l.key.Severity], Action: action, Severity: l.key.Severity,
						Note: index[l.key.Note]}, keys: map[string][]string{}}
					by[l.key.Kind] = a
					order = append(order, a)
				}
				if a.keys[path] == nil {
					a.paths = append(a.paths, path)
				}
				a.keys[path] = append(a.keys[path], l.name)
			}
		}
	}
	n := func(a *acc) int {
		c := 0
		for _, k := range a.keys {
			c += len(k)
		}
		return c
	}
	sort.SliceStable(order, func(i, j int) bool {
		if order[i].need.Severity != order[j].need.Severity {
			return order[i].need.Severity > order[j].need.Severity
		}
		return n(order[i]) < n(order[j])
	})
	out := make([]SummaryNeed, len(order))
	for i, a := range order {
		var parts []string
		for _, p := range a.paths {
			parts = append(parts, p+": "+strings.Join(a.keys[p], ", "))
		}
		a.need.Keys = strings.Join(parts, "; ")
		out[i] = a.need
	}
	return out
}

func keyName(ref string) string {
	_, n, _ := strings.Cut(ref, "/")
	return n
}

// Untouched says what became of the keys that did not go through.
func (s Summary) Untouched() string {
	if s.Running() {
		return ""
	}
	if s.UntouchedText != "" {
		if len(s.Failed()) == 0 {
			return ""
		}
		return s.UntouchedText
	}
	if s.Check {
		return "A dry run changes nothing: every key is as it was."
	}
	if len(s.Failed()) == 0 {
		return ""
	}
	return "Keys marked not rotated were left untouched: the old key is still the active one on those hosts."
}

// Format is the summary as aligned plain text, as `authama ssh rotate` prints
// it at the end of a run: the command, the result, the tree with its
// columns, the notes, what is left for you, and the counts with the command
// that runs the failed keys again.
func (s Summary) Format() string {
	rows := s.Rows()
	w := [4]int{0, len("KEY"), len("VIA"), len("RESULT")}
	reasonW := len("REASON")
	for _, r := range rows {
		if r.Group {
			continue
		}
		w[0] = max(w[0], width(r.Branch)+2+width(r.Label))
		w[1], w[2], w[3] = max(w[1], width(r.Key)), max(w[2], width(r.Via)), max(w[3], width(r.Result))
		reasonW = max(reasonW, width(reasonText(r)))
	}
	total := w[0] + 2 + w[1] + 2 + w[2] + 2 + w[3] + 2 + reasonW
	for _, r := range rows {
		if r.Group {
			total = max(total, width(r.Branch)+width(r.label())+2+width(r.Counts))
		}
	}
	rule := strings.Repeat("─", total)

	var b strings.Builder
	if s.Command != "" {
		fmt.Fprintf(&b, "$ %s\n\n", s.Command)
	}
	noun := "keys"
	if len(s.Keys) == 1 {
		noun = "key"
	}
	owners := "owners"
	if s.Owners() == 1 {
		owners = "owner"
	}
	fmt.Fprintf(&b, "Result   %s   of %d %s · %d %s\n\n", s.Totals(), len(s.Keys), noun, s.Owners(), owners)
	fmt.Fprintf(&b, "%s  %s  %s  %s  %s\n", pad("", w[0]), pad("KEY", w[1]), pad("VIA", w[2]), pad("RESULT", w[3]), "REASON")
	for _, r := range rows {
		if r.Gap {
			b.WriteString(r.Trunk + "\n")
		}
		if r.Group {
			left := r.Branch + r.label()
			if r.Counts == "" {
				b.WriteString(left + "\n")
				continue
			}
			fmt.Fprintf(&b, "%s%s%s\n", left, strings.Repeat(" ", max(2, total-width(left)-width(r.Counts))), r.Counts)
			continue
		}
		line := fmt.Sprintf("%s  %s  %s  %s  %s", pad(r.Branch+r.Mark+" "+r.Label, w[0]), pad(r.Key, w[1]), pad(r.Via, w[2]),
			pad(r.Result, w[3]), reasonText(r))
		b.WriteString(strings.TrimRight(line, " ") + "\n")
	}
	notes := s.Notes()
	if len(notes) > 0 || s.Untouched() != "" {
		b.WriteString("\n" + rule + "\nNotes\n")
		for _, n := range notes {
			lead := fmt.Sprintf("  [%d] ", n.N)
			b.WriteString(lead + wrap(n.Text, total-width(lead), strings.Repeat(" ", width(lead))) + "\n")
		}
		if u := s.Untouched(); u != "" {
			fmt.Fprintf(&b, "  %s\n", u)
		}
	}
	if needs := s.Needs(); len(needs) > 0 {
		b.WriteString(rule + "\nNeeds you\n")
		aw := 0
		for _, n := range needs {
			aw = max(aw, width(n.Action))
		}
		for i, n := range needs {
			ref := ""
			if n.Note > 0 {
				ref = fmt.Sprintf("[%d]", n.Note)
			}
			fmt.Fprintf(&b, "  %s %d  %s  %-4s  %s\n", n.Mark, i+1, pad(n.Action, aw), ref, n.Keys)
		}
	}
	foot := s.Totals()
	if t := s.Took(); t != "" {
		foot += "  ·  " + t
	}
	b.WriteString(rule + "\n")
	if r := s.Rerun(); r != "" {
		right := "re-run failed: " + r
		b.WriteString(foot + strings.Repeat(" ", max(3, total-width(foot)-width(right))) + right + "\n")
	} else {
		b.WriteString(foot + "\n")
	}
	return b.String()
}

func reasonText(r SummaryRow) string {
	if r.Note > 0 {
		return fmt.Sprintf("%s [%d]", r.Reason, r.Note)
	}
	return r.Reason
}

// wrap breaks s into lines of at most n characters at spaces, each line after
// the first indented by hang.
func wrap(s string, n int, hang string) string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		if line != "" && width(line)+1+width(word) > n {
			lines = append(lines, line)
			line = word
			continue
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	lines = append(lines, line)
	return strings.Join(lines, "\n"+hang)
}

func width(s string) int { return utf8.RuneCountInString(s) }

func pad(s string, n int) string { return s + strings.Repeat(" ", max(0, n-width(s))) }

func appendNew(list []string, v string) []string {
	if v == "" {
		return list
	}
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

// failedFile is where the keys a run left are kept, for rotate --failed.
func failedFile(p paths.Paths) string { return filepath.Join(p.StateDir(), "rotate-failed.txt") }

// SaveFailed keeps the keys a run left, one per line, for rotate --failed; an
// empty list clears it.
func SaveFailed(p paths.Paths, refs []string) error {
	if len(refs) == 0 {
		if err := os.Remove(failedFile(p)); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(p.StateDir(), 0o700); err != nil {
		return err
	}
	return os.WriteFile(failedFile(p), []byte(strings.Join(refs, "\n")+"\n"), 0o600)
}

// LoadFailed is the keys the last run left: what rotate --failed runs again.
func LoadFailed(p paths.Paths) ([]string, error) {
	b, err := os.ReadFile(failedFile(p))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(b)), nil
}
