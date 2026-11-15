package selection

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sangehq/authama/src/internal/core/recency"
)

// The transforms: what a selection can do to its rows after the filters and
// before the columns. Each is declarative, with no expression evaluation:
// a template names fields, nothing more.
const (
	// TransformTemplate writes As from a template naming fields in braces:
	// "{profile}/{alias}". "{{" and "}}" are literal braces.
	TransformTemplate = "template"
	// TransformMap replaces Field's value where Values has it: a status
	// needs-redeploy becomes pending. Other values stay unless Default is set.
	TransformMap = "map"
	// TransformFormat rewrites Field: upper, lower, trim, date (Layout, Go's
	// reference layout, 2006-01-02 by default), relative ("3 days ago") or
	// thousands (1,234).
	TransformFormat = "format"
	// TransformSplit makes Field a list, split on Sep (space by default);
	// with Rows, a row per item instead.
	TransformSplit = "split"
	// TransformJoin makes a list Field text, joined by Sep (", " by default).
	TransformJoin = "join"
	// TransformDedupe keeps the first row of each distinct By (every field
	// when By is empty).
	TransformDedupe = "dedupe"
	// TransformGroup makes a row per distinct By, with Aggregates; with
	// Totals, a last row over every group.
	TransformGroup = "group"
	// TransformPivot makes a row per distinct By and a column per distinct
	// value of Field, each cell the first of Aggregates (count by default)
	// over the rows with both; with Totals, a total column and a last row.
	TransformPivot = "pivot"
)

// Transforms lists them, in the order a page offers them.
var Transforms = []string{TransformTemplate, TransformMap, TransformFormat, TransformSplit, TransformJoin, TransformDedupe, TransformGroup, TransformPivot}

// The formats TransformFormat knows.
var Formats = []string{"upper", "lower", "trim", "date", "relative", "thousands"}

// The aggregates a group computes.
var Aggregates = []string{"count", "sum", "min", "max", "list", "first"}

// Transform is one step. Which fields it reads depends on its kind.
type Transform struct {
	Kind string `json:"kind"`
	// Field is the field read (map, format, split, join, and an aggregate's).
	Field string `json:"field,omitempty"`
	// As is the field written; "" rewrites Field. A template needs it.
	As       string            `json:"as,omitempty"`
	Template string            `json:"template,omitempty"`
	Values   map[string]string `json:"values,omitempty"`
	Default  *string           `json:"default,omitempty"` // map: the value for any other
	Format   string            `json:"format,omitempty"`
	Layout   string            `json:"layout,omitempty"`
	Sep      string            `json:"sep,omitempty"`
	Rows     bool              `json:"rows,omitempty"`
	By       []string          `json:"by,omitempty"`
	// Aggregates are a group's computed fields.
	Aggregates []Aggregate `json:"aggregates,omitempty"`
	Totals     bool        `json:"totals,omitempty"`
}

// Aggregate is one computed field of a group: Op over Field, named As
// ("count" needs no field; As defaults to op_field, or count).
type Aggregate struct {
	Op    string `json:"op"`
	Field string `json:"field,omitempty"`
	As    string `json:"as,omitempty"`
}

// Name is the field the aggregate writes.
func (a Aggregate) Name() string {
	switch {
	case a.As != "":
		return a.As
	case a.Field == "":
		return a.Op
	default:
		return a.Op + "_" + a.Field
	}
}

// now is the clock relative dates read; tests set it.
var now = time.Now

// TotalLabel is what a totals row says in its first group column.
const TotalLabel = "total"

// apply runs the transforms over rows whose fields are fields, and answers
// the rows and the fields they now have. Each transform is checked against
// the fields that exist when it runs, so a name it does not know is an
// error naming the step, never a silently empty column.
func apply(ts []Transform, rows []row, fields []string, stage func(string, int)) ([]row, []string, error) {
	for i, t := range ts {
		var err error
		rows, fields, err = t.apply(rows, fields)
		if err != nil {
			return nil, nil, fmt.Errorf("selection: transform %d (%s): %w", i+1, t.Kind, err)
		}
		stage(fmt.Sprintf("transform %d %s", i+1, t.Kind), len(rows))
	}
	return rows, fields, nil
}

func (t Transform) apply(rows []row, fields []string) ([]row, []string, error) {
	has := func(f string) error {
		if f == "" {
			return fmt.Errorf("it needs a field")
		}
		if !slices.Contains(fields, f) {
			return fmt.Errorf("no field %q", f)
		}
		return nil
	}
	out := t.As
	if out == "" {
		out = t.Field
	}
	switch t.Kind {
	case TransformTemplate:
		if t.As == "" {
			return nil, nil, fmt.Errorf("a template needs the field it writes (as)")
		}
		parts, err := parseTemplate(t.Template)
		if err != nil {
			return nil, nil, err
		}
		for _, p := range parts {
			if p.field {
				if err := has(p.text); err != nil {
					return nil, nil, err
				}
			}
		}
		return each(rows, func(f map[string]any) {
			var b strings.Builder
			for _, p := range parts {
				if p.field {
					b.WriteString(Cell(f[p.text]))
				} else {
					b.WriteString(p.text)
				}
			}
			f[t.As] = b.String()
		}), addField(fields, t.As), nil
	case TransformMap:
		if err := has(t.Field); err != nil {
			return nil, nil, err
		}
		return each(rows, func(f map[string]any) {
			v := Cell(f[t.Field])
			switch to, ok := t.Values[v]; {
			case ok:
				f[out] = to
			case t.Default != nil:
				f[out] = *t.Default
			default:
				f[out] = f[t.Field]
			}
		}), addField(fields, out), nil
	case TransformFormat:
		if err := has(t.Field); err != nil {
			return nil, nil, err
		}
		fn, err := formatter(t.Format, t.Layout)
		if err != nil {
			return nil, nil, err
		}
		return each(rows, func(f map[string]any) { f[out] = fn(f[t.Field]) }), addField(fields, out), nil
	case TransformSplit:
		if err := has(t.Field); err != nil {
			return nil, nil, err
		}
		items := func(v any) []string {
			if l, ok := v.([]string); ok {
				return l
			}
			if t.Sep == "" {
				return strings.Fields(Cell(v))
			}
			var l []string
			for _, s := range strings.Split(Cell(v), t.Sep) {
				if s = strings.TrimSpace(s); s != "" {
					l = append(l, s)
				}
			}
			return l
		}
		if !t.Rows {
			return each(rows, func(f map[string]any) { f[out] = items(f[t.Field]) }), addField(fields, out), nil
		}
		var res []row
		for _, r := range rows {
			l := items(r.f[t.Field])
			if len(l) == 0 {
				l = []string{""}
			}
			for _, v := range l {
				c := r.clone()
				c.f[out] = v
				res = append(res, c)
			}
		}
		return res, addField(fields, out), nil
	case TransformJoin:
		if err := has(t.Field); err != nil {
			return nil, nil, err
		}
		sep := t.Sep
		if sep == "" {
			sep = ", "
		}
		return each(rows, func(f map[string]any) {
			if l, ok := f[t.Field].([]string); ok {
				f[out] = strings.Join(l, sep)
			} else {
				f[out] = Cell(f[t.Field])
			}
		}), addField(fields, out), nil
	case TransformDedupe:
		by := t.By
		if len(by) == 0 {
			by = fields
		}
		for _, f := range by {
			if err := has(f); err != nil {
				return nil, nil, err
			}
		}
		seen := map[string]bool{}
		return keep(rows, func(r row) bool {
			k := keyOf(r.f, by)
			if seen[k] {
				return false
			}
			seen[k] = true
			return true
		}), fields, nil
	case TransformGroup:
		return t.group(rows, fields, has)
	case TransformPivot:
		return t.pivot(rows, has)
	case "":
		return nil, nil, fmt.Errorf("a transform needs a kind, one of %s", strings.Join(Transforms, ", "))
	default:
		return nil, nil, fmt.Errorf("no transform %q, one of %s", t.Kind, strings.Join(Transforms, ", "))
	}
}

// group makes a row per distinct By, in the order each first appears.
func (t Transform) group(rows []row, fields []string, has func(string) error) ([]row, []string, error) {
	if len(t.By) == 0 {
		return nil, nil, fmt.Errorf("a group needs the fields it groups by (by)")
	}
	for _, f := range t.By {
		if err := has(f); err != nil {
			return nil, nil, err
		}
	}
	out := slices.Clone(t.By)
	for _, a := range t.Aggregates {
		if !slices.Contains(Aggregates, a.Op) {
			return nil, nil, fmt.Errorf("no aggregate %q, one of %s", a.Op, strings.Join(Aggregates, ", "))
		}
		if a.Op != "count" {
			if err := has(a.Field); err != nil {
				return nil, nil, fmt.Errorf("%s: %w", a.Op, err)
			}
		}
		if slices.Contains(out, a.Name()) {
			return nil, nil, fmt.Errorf("two columns named %q", a.Name())
		}
		out = append(out, a.Name())
	}
	var order []string
	members := map[string][]row{}
	for _, r := range rows {
		k := keyOf(r.f, t.By)
		if _, ok := members[k]; !ok {
			order = append(order, k)
		}
		members[k] = append(members[k], r)
	}
	res := make([]row, 0, len(order)+1)
	for _, k := range order {
		m := members[k]
		f := map[string]any{}
		for _, b := range t.By {
			f[b] = m[0].f[b]
		}
		for _, a := range t.Aggregates {
			f[a.Name()] = aggregate(a, m)
		}
		res = append(res, row{f: f})
	}
	if t.Totals {
		f := map[string]any{t.By[0]: TotalLabel}
		for _, a := range t.Aggregates {
			f[a.Name()] = aggregate(a, rows)
		}
		res = append(res, row{f: f, total: true})
	}
	return res, out, nil
}

// PivotEmpty is the column a pivot gives rows whose Field is empty.
const PivotEmpty = "(none)"

// pivot makes a row per distinct By, in the order each first appears, and
// a column per distinct value of Field, sorted.
func (t Transform) pivot(rows []row, has func(string) error) ([]row, []string, error) {
	if len(t.By) == 0 {
		return nil, nil, fmt.Errorf("a pivot needs the fields its rows are by (by)")
	}
	if err := has(t.Field); err != nil {
		return nil, nil, fmt.Errorf("its columns: %w", err)
	}
	for _, f := range t.By {
		if err := has(f); err != nil {
			return nil, nil, err
		}
		if f == t.Field {
			return nil, nil, fmt.Errorf("%q cannot be both its rows and its columns", f)
		}
	}
	agg := Aggregate{Op: "count"}
	switch len(t.Aggregates) {
	case 0:
	case 1:
		agg = t.Aggregates[0]
	default:
		return nil, nil, fmt.Errorf("a pivot's cells are one aggregate, not %d", len(t.Aggregates))
	}
	if !slices.Contains(Aggregates, agg.Op) {
		return nil, nil, fmt.Errorf("no aggregate %q, one of %s", agg.Op, strings.Join(Aggregates, ", "))
	}
	if agg.Op != "count" {
		if err := has(agg.Field); err != nil {
			return nil, nil, fmt.Errorf("%s: %w", agg.Op, err)
		}
	}
	colOf := func(r row) []string {
		vs, ok := r.f[t.Field].([]string)
		if !ok {
			vs = []string{Cell(r.f[t.Field])}
		}
		out := make([]string, 0, len(vs))
		for _, v := range vs {
			if v == "" {
				v = PivotEmpty
			}
			out = append(out, v)
		}
		if len(out) == 0 {
			out = []string{PivotEmpty}
		}
		return out
	}
	var cols, order []string
	members := map[string][]row{}          // a row key's rows
	cells := map[string]map[string][]row{} // a row key's rows in each column
	for _, r := range rows {
		k := keyOf(r.f, t.By)
		if _, ok := cells[k]; !ok {
			cells[k] = map[string][]row{}
			order = append(order, k)
		}
		members[k] = append(members[k], r)
		for _, c := range colOf(r) {
			if !slices.Contains(cols, c) {
				cols = append(cols, c)
			}
			cells[k][c] = append(cells[k][c], r)
		}
	}
	sort.Strings(cols)
	out := slices.Clone(t.By)
	for _, c := range cols {
		if slices.Contains(out, c) {
			return nil, nil, fmt.Errorf("the value %q of %s is also the name of a field it is by", c, t.Field)
		}
		out = append(out, c)
	}
	if t.Totals {
		out = append(out, TotalLabel)
	}
	cell := func(rs []row) any {
		if len(rs) == 0 && agg.Op != "count" && agg.Op != "sum" {
			return nil
		}
		return aggregate(agg, rs)
	}
	res := make([]row, 0, len(order)+1)
	for _, k := range order {
		f := map[string]any{}
		for _, b := range t.By {
			f[b] = members[k][0].f[b]
		}
		for _, c := range cols {
			f[c] = cell(cells[k][c])
		}
		if t.Totals {
			f[TotalLabel] = cell(members[k])
		}
		res = append(res, row{f: f})
	}
	if t.Totals {
		f := map[string]any{t.By[0]: TotalLabel}
		for _, c := range cols {
			var rs []row
			for _, k := range order {
				rs = append(rs, cells[k][c]...)
			}
			f[c] = cell(rs)
		}
		f[TotalLabel] = cell(rows)
		res = append(res, row{f: f, total: true})
	}
	return res, out, nil
}

func aggregate(a Aggregate, rows []row) any {
	switch a.Op {
	case "count":
		return len(rows)
	case "sum":
		n := 0
		for _, r := range rows {
			n += number(r.f[a.Field])
		}
		return n
	case "first":
		if len(rows) == 0 {
			return nil
		}
		return rows[0].f[a.Field]
	case "list":
		var l []string
		for _, r := range rows {
			vs, ok := r.f[a.Field].([]string)
			if !ok {
				vs = []string{Cell(r.f[a.Field])}
			}
			for _, v := range vs {
				if v != "" && !slices.Contains(l, v) {
					l = append(l, v)
				}
			}
		}
		if l == nil {
			l = []string{}
		}
		return l
	}
	// min and max, numeric when every value is, else by text.
	var best any
	for _, r := range rows {
		v := r.f[a.Field]
		if v == nil || Cell(v) == "" {
			continue
		}
		if best == nil {
			best = v
			continue
		}
		if c := compare(v, best); (a.Op == "min" && c < 0) || (a.Op == "max" && c > 0) {
			best = v
		}
	}
	return best
}

// number reads an int field, or text holding one; anything else counts 0.
func number(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case bool:
		if x {
			return 1
		}
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(Cell(v)))
	return n
}

func formatter(format, layout string) (func(any) any, error) {
	switch format {
	case "upper":
		return func(v any) any { return mapText(v, strings.ToUpper) }, nil
	case "lower":
		return func(v any) any { return mapText(v, strings.ToLower) }, nil
	case "trim":
		return func(v any) any { return mapText(v, strings.TrimSpace) }, nil
	case "date":
		if layout == "" {
			layout = "2006-01-02"
		}
		return func(v any) any {
			if t, ok := parseTime(Cell(v)); ok {
				return t.Format(layout)
			}
			return v
		}, nil
	case "relative":
		return func(v any) any {
			if t, ok := parseTime(Cell(v)); ok {
				return relative(now().Sub(t))
			}
			return v
		}, nil
	case "thousands":
		return func(v any) any {
			s := Cell(v)
			n, err := strconv.Atoi(s)
			if err != nil {
				return v
			}
			return thousands(n)
		}, nil
	case "":
		return nil, fmt.Errorf("a format needs one of %s", strings.Join(Formats, ", "))
	default:
		return nil, fmt.Errorf("no format %q, one of %s", format, strings.Join(Formats, ", "))
	}
}

// mapText applies fn to text, or to each item of a list.
func mapText(v any, fn func(string) string) any {
	if l, ok := v.([]string); ok {
		out := make([]string, len(l))
		for i, s := range l {
			out[i] = fn(s)
		}
		return out
	}
	if s, ok := v.(string); ok {
		return fn(s)
	}
	return v
}

// parseTime reads the times the dataset holds: RFC 3339, or a bare date.
func parseTime(s string) (time.Time, bool) {
	for _, l := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(l, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// relative words a duration; recency.Ago is the one implementation.
func relative(d time.Duration) string { return recency.Ago(d) }

func thousands(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

type templatePart struct {
	text  string
	field bool
}

func parseTemplate(s string) ([]templatePart, error) {
	if s == "" {
		return nil, fmt.Errorf("an empty template")
	}
	var parts []templatePart
	var lit strings.Builder
	for i := 0; i < len(s); i++ {
		switch {
		case strings.HasPrefix(s[i:], "{{"):
			lit.WriteByte('{')
			i++
		case strings.HasPrefix(s[i:], "}}"):
			lit.WriteByte('}')
			i++
		case s[i] == '{':
			end := strings.IndexByte(s[i:], '}')
			if end < 0 {
				return nil, fmt.Errorf("an unclosed { in %q", s)
			}
			name := strings.TrimSpace(s[i+1 : i+end])
			if name == "" {
				return nil, fmt.Errorf("an empty {} in %q", s)
			}
			if lit.Len() > 0 {
				parts = append(parts, templatePart{text: lit.String()})
				lit.Reset()
			}
			parts = append(parts, templatePart{text: name, field: true})
			i += end
		case s[i] == '}':
			return nil, fmt.Errorf("a } with no { in %q (write }} for a brace)", s)
		default:
			lit.WriteByte(s[i])
		}
	}
	if lit.Len() > 0 {
		parts = append(parts, templatePart{text: lit.String()})
	}
	return parts, nil
}

// each runs fn over a copy of every row's fields: a step answers new rows
// and leaves the ones it was given as they were.
func each(rows []row, fn func(map[string]any)) []row {
	out := make([]row, len(rows))
	for i, r := range rows {
		c := r.clone()
		fn(c.f)
		out[i] = c
	}
	return out
}

func (r row) clone() row {
	f := make(map[string]any, len(r.f)+1)
	for k, v := range r.f {
		f[k] = v
	}
	return row{f: f, keys: r.keys, total: r.total}
}

func addField(fields []string, f string) []string {
	if slices.Contains(fields, f) {
		return fields
	}
	return append(slices.Clone(fields), f)
}

func keyOf(f map[string]any, by []string) string {
	parts := make([]string, len(by))
	for i, b := range by {
		parts[i] = Cell(f[b])
	}
	return strings.Join(parts, "\x00")
}
