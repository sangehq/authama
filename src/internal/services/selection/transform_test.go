package selection

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func hostsWith(t *testing.T, cols []Column, ts ...Transform) Result {
	t.Helper()
	ds, _, _ := build(t)
	res, err := Run(ds, Selection{Version: Version, Grain: GrainHost, Transforms: ts, Columns: cols,
		Sort: []Sort{{Field: "alias"}}})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func col(r Result, name string) []string {
	i := -1
	for j, c := range r.Columns {
		if c == name {
			i = j
		}
	}
	var out []string
	for _, row := range r.Rows {
		if i < 0 {
			out = append(out, "<no column>")
			continue
		}
		out = append(out, Cell(row[i]))
	}
	return out
}

func TestATemplateWritesAColumnFromFields(t *testing.T) {
	r := hostsWith(t, []Column{{Field: "alias"}, {Field: "ssh"}},
		Transform{Kind: TransformTemplate, As: "ssh", Template: "ssh {user}@{hostname} -p {port} {{x}}"})
	if got := col(r, "ssh"); got[0] != "ssh ploi@203.0.113.20 -p 22 {x}" || len(got) != 5 {
		t.Errorf("ssh = %q", got)
	}
}

func TestAMapRewritesKnownValues(t *testing.T) {
	other := "elsewhere"
	r := hostsWith(t, []Column{{Field: "alias"}, {Field: "where"}},
		Transform{Kind: TransformMap, Field: "profile", As: "where", Values: map[string]string{"simtabi": "ours"}, Default: &other})
	want := []string{"elsewhere", "elsewhere", "ours", "ours", "ours"} // dani, gh, oribi, simbi-db, simbi-web
	if got := col(r, "where"); !reflect.DeepEqual(got, want) {
		t.Errorf("where = %q, want %q", got, want)
	}
	// Without a default, other values stay; with no as, the field is rewritten.
	r = hostsWith(t, []Column{{Field: "profile"}}, Transform{Kind: TransformMap, Field: "profile", Values: map[string]string{"work": "WORK"}})
	if got := col(r, "profile"); got[1] != "WORK" || got[0] != "adelsaiq" {
		t.Errorf("profile = %q", got)
	}
}

func TestFormatsRewriteTextDatesAndNumbers(t *testing.T) {
	r := hostsWith(t, []Column{{Field: "alias"}, {Field: "tags"}},
		Transform{Kind: TransformFormat, Field: "alias", Format: "upper"},
		Transform{Kind: TransformFormat, Field: "tags", Format: "upper"})
	if got := col(r, "alias"); got[0] != "DANI-STG-WEB-00" {
		t.Errorf("upper alias = %q", got)
	}
	if got := col(r, "tags"); got[0] != "WEB STG" {
		t.Errorf("upper tags = %q", got)
	}

	defer func(f func() time.Time) { now = f }(now)
	now = func() time.Time { return time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC) }
	for _, c := range []struct {
		format, layout string
		in             any
		want           string
	}{
		{"date", "", "2026-11-05T10:00:00Z", "2026-11-05"},
		{"date", "Jan 2 2006", "2026-11-05", "Nov 5 2026"},
		{"date", "", "not a date", "not a date"},
		{"relative", "", "2026-11-05T12:00:00Z", "1 day ago"},
		{"relative", "", "2026-11-03", "3 days ago"},
		{"relative", "", "2026-11-06T11:59:30Z", "just now"},
		{"relative", "", "2026-12-07T12:00:00Z", "in 1 month"},
		{"thousands", "", 1234567, "1,234,567"},
		{"thousands", "", -1234, "-1,234"},
		{"thousands", "", 999, "999"},
		{"trim", "", "  x ", "x"},
		{"lower", "", "AbC", "abc"},
	} {
		fn, err := formatter(c.format, c.layout)
		if err != nil {
			t.Fatal(err)
		}
		if got := Cell(fn(c.in)); got != c.want {
			t.Errorf("%s %q of %v = %q, want %q", c.format, c.layout, c.in, got, c.want)
		}
	}
}

func TestSplitAndJoin(t *testing.T) {
	r := hostsWith(t, []Column{{Field: "alias"}, {Field: "tag"}},
		Transform{Kind: TransformSplit, Field: "tags", As: "tag", Rows: true})
	// dani: web stg; gh: none; oribi: none; simbi-db: db prd; simbi-web: web prd.
	if got := col(r, "tag"); len(got) != 8 || got[0] != "web" || got[1] != "stg" || got[2] != "" {
		t.Errorf("one row per tag: %q", got)
	}
	r = hostsWith(t, []Column{{Field: "alias"}, {Field: "tags"}},
		Transform{Kind: TransformJoin, Field: "tags", Sep: "|"})
	if got := col(r, "tags"); got[0] != "web|stg" {
		t.Errorf("joined = %q", got)
	}
	r = hostsWith(t, []Column{{Field: "alias"}, {Field: "parts"}},
		Transform{Kind: TransformSplit, Field: "alias", As: "parts", Sep: "-"},
		Transform{Kind: TransformJoin, Field: "parts", Sep: " / "})
	if got := col(r, "parts"); got[0] != "dani / stg / web / 00" {
		t.Errorf("split then join = %q", got)
	}
}

func TestDedupeKeepsTheFirstOfEach(t *testing.T) {
	r := hostsWith(t, []Column{{Field: "profile"}, {Field: "alias"}},
		Transform{Kind: TransformDedupe, By: []string{"profile"}})
	if got := col(r, "profile"); len(got) != 3 {
		t.Errorf("one per profile: %q", got)
	}
	if got := r.Stages[len(r.Stages)-1]; got.Name != "transform 1 dedupe" || got.Rows != 3 {
		t.Errorf("stage = %+v", got)
	}
}

func TestAGroupAggregatesWithATotalsRow(t *testing.T) {
	ds, _, _ := build(t)
	res, err := Run(ds, Selection{Version: Version, Grain: GrainHost,
		Transforms: []Transform{{Kind: TransformGroup, By: []string{"profile"}, Totals: true, Aggregates: []Aggregate{
			{Op: "count"}, {Op: "sum", Field: "port"}, {Op: "min", Field: "alias"}, {Op: "max", Field: "alias"},
			{Op: "list", Field: "tags"}, {Op: "first", Field: "user", As: "user"}}}},
		Sort: []Sort{{Field: "count", Desc: true}}, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"profile", "count", "sum_port", "min_alias", "max_alias", "list_tags", "user"}
	if !reflect.DeepEqual(res.Columns, want) {
		t.Fatalf("columns = %q, want %q (a group's fields alone)", res.Columns, want)
	}
	got := make([]string, len(res.Rows))
	for i, r := range res.Rows {
		got[i] = strings.Join(Cells(r), ",")
	}
	wantRows := []string{
		"simtabi,3,66,oribi-web,simbi-prd-web-00,web prd db,ploi",
		"adelsaiq,1,22,dani-stg-web-00,dani-stg-web-00,web stg,ploi",
		"total,5,110,dani-stg-web-00,simbi-prd-web-00,web prd db stg,ploi",
	}
	if !reflect.DeepEqual(got, wantRows) {
		t.Errorf("rows (sorted, limited to 2, totals kept last):\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(wantRows, "\n"))
	}
}

func TestAColumnCanNameATransformedField(t *testing.T) {
	ds, _, _ := build(t)
	_, err := Run(ds, Selection{Version: Version, Grain: GrainHost, Columns: []Column{{Field: "ssh"}}})
	if err == nil {
		t.Fatal("a column no transform wrote was accepted")
	}
	// Without columns of its own, a selection gets the defaults and the new field.
	res := hostsWith(t, nil, Transform{Kind: TransformTemplate, As: "where", Template: "{profile}/{alias}"})
	if last := res.Columns[len(res.Columns)-1]; last != "where" || len(res.Columns) != len(Defaults[GrainHost])+1 {
		t.Errorf("columns = %q", res.Columns)
	}
}

func TestABadTransformSaysWhichAndWhy(t *testing.T) {
	ds, _, _ := build(t)
	for want, tr := range map[string]Transform{
		"transform 1 (template): a template needs the field it writes": {Kind: TransformTemplate, Template: "{alias}"},
		`no field "nope"`:               {Kind: TransformTemplate, As: "x", Template: "{nope}"},
		"an unclosed {":                 {Kind: TransformTemplate, As: "x", Template: "{alias"},
		"a } with no {":                 {Kind: TransformTemplate, As: "x", Template: "a}"},
		"a transform needs a kind":      {},
		`no transform "sql"`:            {Kind: "sql"},
		`no format "rot13"`:             {Kind: TransformFormat, Field: "alias", Format: "rot13"},
		"it needs a field":              {Kind: TransformMap},
		"a group needs the fields":      {Kind: TransformGroup},
		`no aggregate "avg"`:            {Kind: TransformGroup, By: []string{"profile"}, Aggregates: []Aggregate{{Op: "avg", Field: "port"}}},
		`two columns named "profile"`:   {Kind: TransformGroup, By: []string{"profile"}, Aggregates: []Aggregate{{Op: "first", Field: "alias", As: "profile"}}},
		`sum: no field "weight"`:        {Kind: TransformGroup, By: []string{"profile"}, Aggregates: []Aggregate{{Op: "sum", Field: "weight"}}},
		`dedupe): no field "elsewhere"`: {Kind: TransformDedupe, By: []string{"elsewhere"}},
	} {
		_, err := Run(ds, Selection{Version: Version, Grain: GrainHost, Transforms: []Transform{tr}})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%+v: %v, want %q", tr, err, want)
		}
	}
	// A later step sees the fields an earlier one wrote, and not the reverse.
	_, err := Run(ds, Selection{Version: Version, Grain: GrainHost, Transforms: []Transform{
		{Kind: TransformFormat, Field: "where", Format: "upper"},
		{Kind: TransformTemplate, As: "where", Template: "{alias}"}}})
	if err == nil || !strings.Contains(err.Error(), "transform 1") {
		t.Errorf("a step reading a field written later: %v", err)
	}
}

// Transforms round-trip through a saved selection, and Parse still refuses
// a field it does not know.
func TestTransformsParse(t *testing.T) {
	in := `{"version":1,"grain":"host","transforms":[{"kind":"group","by":["profile"],"aggregates":[{"op":"count"}],"totals":true}]}`
	s, err := Parse([]byte(in))
	if err != nil || len(s.Transforms) != 1 || s.Transforms[0].Aggregates[0].Name() != "count" {
		t.Fatalf("%+v %v", s, err)
	}
	b, _ := json.Marshal(s)
	if again, err := Parse(b); err != nil || !reflect.DeepEqual(again, s) {
		t.Errorf("round trip: %+v %v", again, err)
	}
	if _, err := Parse([]byte(`{"version":1,"grain":"host","transforms":[{"kind":"map","expr":"x"}]}`)); err == nil {
		t.Error("an unknown transform field was accepted")
	}
}

// A pivot: a row per profile, a column per tag (a list field puts a host in
// each of its tags' columns), cells counting hosts, and totals both ways
// that count each host once.
func TestAPivotCountsRowsByTwoFields(t *testing.T) {
	ds, _, _ := build(t)
	res, err := Run(ds, Selection{Version: Version, Grain: GrainHost,
		Transforms: []Transform{{Kind: TransformPivot, By: []string{"profile"}, Field: "tags", Totals: true}},
		Sort:       []Sort{{Field: "profile"}}})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"profile", PivotEmpty, "db", "prd", "stg", "web", TotalLabel}; !reflect.DeepEqual(res.Columns, want) {
		t.Fatalf("columns = %q, want %q", res.Columns, want)
	}
	var got []string
	for _, r := range res.Rows {
		got = append(got, strings.Join(Cells(r), ","))
	}
	want := []string{
		"adelsaiq,0,0,0,1,1,1",
		"simtabi,1,1,2,0,1,3",
		"work,1,0,0,0,0,1",
		"total,2,1,2,1,2,5",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rows:\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// Another aggregate: the cells sum the ports.
	res, err = Run(ds, Selection{Version: Version, Grain: GrainHost,
		Transforms: []Transform{{Kind: TransformPivot, By: []string{"profile"}, Field: "user", Aggregates: []Aggregate{{Op: "sum", Field: "port"}}}},
		Sort:       []Sort{{Field: "profile"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := Cells(res.Rows[1]); strings.Join(res.Columns, ",") != "profile,git,ploi" || strings.Join(got, ",") != "simtabi,0,66" {
		t.Errorf("sum: %q %q", res.Columns, got)
	}
	for want, tr := range map[string]Transform{
		"a pivot needs the fields":     {Kind: TransformPivot, Field: "tags"},
		`its columns: no field "nope"`: {Kind: TransformPivot, By: []string{"profile"}, Field: "nope"},
		"cannot be both its rows":      {Kind: TransformPivot, By: []string{"profile"}, Field: "profile"},
		"one aggregate, not 2":         {Kind: TransformPivot, By: []string{"profile"}, Field: "user", Aggregates: []Aggregate{{Op: "count"}, {Op: "count"}}},
		`no aggregate "avg"`:           {Kind: TransformPivot, By: []string{"profile"}, Field: "user", Aggregates: []Aggregate{{Op: "avg"}}},
	} {
		_, err := Run(ds, Selection{Version: Version, Grain: GrainHost, Transforms: []Transform{tr}})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%+v: %v, want %q", tr, err, want)
		}
	}
	// A column named like a field the rows are by: every host's x is "user".
	_, err = Run(ds, Selection{Version: Version, Grain: GrainHost, Transforms: []Transform{
		{Kind: TransformTemplate, As: "x", Template: "user"}, {Kind: TransformPivot, By: []string{"user"}, Field: "x"}}})
	if err == nil || !strings.Contains(err.Error(), "also the name of a field it is by") {
		t.Errorf("clash: %v", err)
	}
}
