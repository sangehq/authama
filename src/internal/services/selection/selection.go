// Package selection runs a selection over the export dataset: which rows
// (the grain), which of them (facets and text), what each row says (columns,
// following relations by path), in what order and how many. It is the
// second stage of the export pipeline (dataset, selection, format), and the
// same Selection value drives the web page, the API and the CLI.
package selection

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/simtabi/authama/src/internal/services/dataset"
)

// Version is the Selection shape this package reads. A saved selection of
// another version is refused, never half-read.
const Version = 1

// The grains: what one row of the result is.
const (
	GrainProfile = "profile"
	GrainKey     = "key"
	GrainHost    = "host"
	GrainToken   = "token"
	// The records: history, the events log, snapshots and deployments.
	GrainOp         = "op"
	GrainEvent      = "event"
	GrainSnapshot   = "snapshot"
	GrainDeployment = "deployment"
)

// Grains lists them, in the order a page offers them.
var Grains = []string{GrainProfile, GrainKey, GrainHost, GrainToken, GrainOp, GrainEvent, GrainSnapshot, GrainDeployment}

// Selection is what an export holds.
type Selection struct {
	Version int    `json:"version"`
	Grain   string `json:"grain"`
	Where   Where  `json:"where"`
	// Narrow makes a row's lists and counts cover only what passed the
	// facets: a profile kept for a key lists that key alone.
	Narrow  bool     `json:"narrow,omitempty"`
	Columns []Column `json:"columns,omitempty"` // empty: the grain's default columns
	Sort    []Sort   `json:"sort,omitempty"`
	Limit   int      `json:"limit,omitempty"` // 0: every row
}

// Where holds the facets. An empty list is no filter; a value in a list is
// matched exactly. Include and Exclude match any column, ignoring case.
type Where struct {
	Profile []string `json:"profile,omitempty"`
	Key     []string `json:"key,omitempty"` // a key's name or its profile/name ref
	Env     []string `json:"env,omitempty"`
	Role    []string `json:"role,omitempty"`
	Tag     []string `json:"tag,omitempty"`
	Status  []string `json:"status,omitempty"` // a host's status, a key's state
	Target  []string `json:"target,omitempty"` // a token's tool
	Kind    []string `json:"kind,omitempty"`   // an event's kind (added, changed, ...), a token's kind
	Area    []string `json:"area,omitempty"`   // an event's area (ssh, config, auth)
	Source  []string `json:"source,omitempty"` // an operation's source (cli, web, ...)
	// Command keeps operations whose command is one of these, or starts with
	// one and a space ("host" keeps "host add").
	Command []string `json:"command,omitempty"`
	// Since and Until bound a row's time (an operation's start, an event's
	// time, a snapshot's, a deployment's date), inclusive; a prefix such as
	// 2026-11 or 2026-11-06 works.
	Since   string `json:"since,omitempty"`
	Until   string `json:"until,omitempty"`
	Include string `json:"include,omitempty"`
	Exclude string `json:"exclude,omitempty"`
}

// Column is one column of the result: a field, and its heading.
type Column struct {
	Field string `json:"field"`
	As    string `json:"as,omitempty"` // "" is the field's own name
}

// Heading is the column's heading.
func (c Column) Heading() string {
	if c.As != "" {
		return c.As
	}
	return c.Field
}

// Sort is one sort key.
type Sort struct {
	Field string `json:"field"`
	Desc  bool   `json:"desc,omitempty"`
}

// Stage is how many rows were left after one step, so a page can say which
// step left rows out.
type Stage struct {
	Name string `json:"name"` // all, profile, key, facets, text
	Rows int    `json:"rows"`
}

// Result is a selection's rows, typed: strings, ints, bools and lists of
// strings. Cells formats them as text.
type Result struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
	Stages  []Stage  `json:"stages"`
	// Fields is every field the grain offers, for a page to choose columns from.
	Fields []string `json:"fields"`
}

// Parse reads a selection, refusing another version.
func Parse(b []byte) (Selection, error) {
	var s Selection
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&s); err != nil {
		return Selection{}, fmt.Errorf("selection: %w", err)
	}
	if s.Version != Version {
		return Selection{}, fmt.Errorf("selection: version %d, this authama reads version %d", s.Version, Version)
	}
	return s, nil
}

// row is one candidate row: its fields by name, and what the facets match on.
type row struct {
	f map[string]any
	// keys are the keys the row holds or uses, by name and by ref; narrowing
	// keeps the ones a key facet chose.
	keys []dataset.Key
}

// Run runs s over ds.
func Run(ds *dataset.Dataset, s Selection) (Result, error) {
	if s.Version != 0 && s.Version != Version {
		return Result{}, fmt.Errorf("selection: version %d, this authama reads version %d", s.Version, Version)
	}
	rows, fields, err := rowsOf(ds, s.Grain)
	if err != nil {
		return Result{}, err
	}
	cols := s.Columns
	if len(cols) == 0 {
		cols = Defaults[s.Grain]
	}
	for _, c := range cols {
		if !slices.Contains(fields, c.Field) {
			return Result{}, fmt.Errorf("selection: %s rows have no field %q", s.Grain, c.Field)
		}
	}
	res := Result{Fields: fields, Stages: []Stage{{"all", len(rows)}}}
	w := s.Where

	rows = keep(rows, func(r row) bool { return in(w.Profile, str(r.f["profile"])) })
	res.Stages = append(res.Stages, Stage{"profile", len(rows)})

	rows = byKey(rows, w.Key, s.Narrow)
	res.Stages = append(res.Stages, Stage{"key", len(rows)})

	rows = keep(rows, func(r row) bool {
		return in(w.Env, str(r.f["env"])) && in(w.Role, str(r.f["role"])) && anyIn(w.Tag, list(r.f["tags"])) &&
			in(w.Status, str(r.f["status"])) && in(w.Target, str(r.f["target"])) &&
			in(w.Kind, str(r.f["kind"])) && in(w.Area, str(r.f["area"])) && in(w.Source, str(r.f["source"])) &&
			commandIn(w.Command, str(r.f["command"])) && within(str(r.f["time"]), w.Since, w.Until)
	})
	res.Stages = append(res.Stages, Stage{"facets", len(rows)})

	// Text matches the columns the file has, and the grain's own defaults, so
	// a column left out can still choose rows, as the CSV filters always did.
	textCols := append(append([]Column{}, Defaults[s.Grain]...), cols...)
	rows = keep(rows, func(r row) bool {
		line := strings.ToLower(strings.Join(cells(r, textCols), "\x00"))
		if inc := strings.ToLower(strings.TrimSpace(w.Include)); inc != "" && !strings.Contains(line, inc) {
			return false
		}
		exc := strings.ToLower(strings.TrimSpace(w.Exclude))
		return exc == "" || !strings.Contains(line, exc)
	})
	res.Stages = append(res.Stages, Stage{"text", len(rows)})

	if len(s.Sort) > 0 {
		sort.SliceStable(rows, func(i, j int) bool {
			for _, k := range s.Sort {
				c := compare(rows[i].f[k.Field], rows[j].f[k.Field])
				if c != 0 {
					return (c < 0) != k.Desc
				}
			}
			return false
		})
	}
	if s.Limit > 0 && len(rows) > s.Limit {
		rows = rows[:s.Limit]
	}
	for _, c := range cols {
		res.Columns = append(res.Columns, c.Heading())
	}
	res.Rows = [][]any{}
	for _, r := range rows {
		out := make([]any, len(cols))
		for i, c := range cols {
			out[i] = r.f[c.Field]
		}
		res.Rows = append(res.Rows, out)
	}
	return res, nil
}

// byKey keeps the rows holding or using a chosen key; with narrow, a row's
// key lists and counts are cut down to the chosen keys.
func byKey(rows []row, chosen []string, narrow bool) []row {
	if len(chosen) == 0 {
		return rows
	}
	want := map[string]bool{}
	for _, k := range chosen {
		want[k] = true
	}
	var out []row
	for _, r := range rows {
		var hit []dataset.Key
		for _, k := range r.keys {
			if want[k.Name] || want[k.Ref] {
				hit = append(hit, k)
			}
		}
		if len(hit) == 0 {
			continue
		}
		if narrow && r.f["key_names"] != nil {
			f := map[string]any{}
			for k, v := range r.f {
				f[k] = v
			}
			names, hosts := make([]string, 0, len(hit)), 0
			for _, k := range hit {
				names = append(names, k.Name)
				hosts += len(k.Hosts)
			}
			f["key_names"], f["keys"], f["hosts"] = names, len(hit), hosts
			r = row{f: f, keys: hit}
		}
		out = append(out, r)
	}
	return out
}

func keep(rows []row, ok func(row) bool) []row {
	out := rows[:0:0]
	for _, r := range rows {
		if ok(r) {
			out = append(out, r)
		}
	}
	return out
}

// commandIn is true when list is empty, or v is one of its commands or a
// subcommand of one.
func commandIn(list []string, v string) bool {
	if len(list) == 0 {
		return true
	}
	for _, c := range list {
		if v == c || strings.HasPrefix(v, c+" ") {
			return true
		}
	}
	return false
}

// within is true when t is between since and until, inclusive, comparing
// as many characters as each bound has, so "2026-11" covers the month.
func within(t, since, until string) bool {
	if since != "" && (t == "" || t[:min(len(t), len(since))] < since) {
		return false
	}
	if until != "" && (t == "" || t[:min(len(t), len(until))] > until) {
		return false
	}
	return true
}

// in is true when list is empty or holds v.
func in(list []string, v string) bool { return len(list) == 0 || slices.Contains(list, v) }

// anyIn is true when list is empty or shares a value with vs.
func anyIn(list, vs []string) bool {
	if len(list) == 0 {
		return true
	}
	for _, v := range vs {
		if slices.Contains(list, v) {
			return true
		}
	}
	return false
}

func str(v any) string { return Cell(v) }

func list(v any) []string {
	if l, ok := v.([]string); ok {
		return l
	}
	return nil
}

// Cell is a value as text: lists joined by spaces, as the CSVs always wrote them.
func Cell(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	case bool:
		return strconv.FormatBool(x)
	case []string:
		return strings.Join(x, " ")
	default:
		return fmt.Sprint(x)
	}
}

// Cells is a result row as text.
func Cells(r []any) []string {
	out := make([]string, len(r))
	for i, v := range r {
		out[i] = Cell(v)
	}
	return out
}

func cells(r row, cols []Column) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = Cell(r.f[c.Field])
	}
	return out
}

func compare(a, b any) int {
	if x, ok := a.(int); ok {
		if y, ok := b.(int); ok {
			return x - y
		}
	}
	return strings.Compare(strings.ToLower(Cell(a)), strings.ToLower(Cell(b)))
}

// ErrUnknownGrain is a grain this package does not have.
var ErrUnknownGrain = errors.New("selection: unknown grain")
