package encode

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/sangehq/authama/src/internal/util/csvout"
)

// sample holds every hazard a format must survive: a spreadsheet formula, a
// quote and a backslash (PHP), markup (HTML), a pipe and a line break
// (Markdown), and values that look like other types (YAML).
func sample() Table {
	return Table{
		Columns: []string{"alias", "port", "vpn", "tags", "note", "mode"},
		Rows: [][]any{
			{"simbi-prd-web-00", 22, false, []string{"web", "prd"}, `=HYPERLINK("x")`, "0600"},
			{"o'brien\\box", int64(2222), true, []string{}, "a | b\nc </td><script>alert(1)</script>", "true"},
		},
	}
}

func write(t *testing.T, name string, tb Table, o Options) string {
	t.Helper()
	f, err := Get(name)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := f.Write(&buf, tb, o); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return buf.String()
}

func TestEveryFormatIsRegisteredAndWrites(t *testing.T) {
	want := []string{"csv", "html", "js", "json", "jsonl", "markdown", "php", "tsv", "yaml"}
	if got := TableNames(); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("table formats = %v, want %v", got, want)
	}
	hosts := []string{"hosts-csv", "iterm2", "mobaxterm", "putty", "remmina", "securecrt", "ssh_config", "termius", "windows-terminal"}
	if got := HostNames(); strings.Join(got, ",") != strings.Join(hosts, ",") {
		t.Fatalf("host formats = %v, want %v", got, hosts)
	}
	if len(Names()) != len(want)+len(hosts) {
		t.Fatalf("Names() = %v", Names())
	}
	for _, n := range want {
		f, _ := Get(n)
		if f.Extension == "" || f.MediaType == "" || f.Label == "" {
			t.Errorf("%s lacks an extension, media type or label", n)
		}
		if out := write(t, n, sample(), Options{}); !strings.Contains(out, "simbi-prd-web-00") {
			t.Errorf("%s did not write the first row:\n%s", n, out)
		}
		// An empty table still writes a well-formed file.
		_ = write(t, n, Table{Columns: []string{"a"}}, Options{})
	}
	if _, err := Get("xlsx"); err == nil {
		t.Error("an unknown format was accepted")
	}
}

// CSV is csvout's, byte for byte, so the lists' CSVs cannot drift.
func TestCSVIsTheListsCSVAndFormulaSafe(t *testing.T) {
	tb := sample()
	ct := csvout.Table{Header: tb.Columns}
	for _, r := range tb.Rows {
		ct.Rows = append(ct.Rows, texts(r, len(tb.Columns)))
	}
	var want bytes.Buffer
	_ = csvout.Write(&want, ct)
	got := write(t, "csv", tb, Options{})
	if got != want.String() {
		t.Errorf("csv differs from csvout:\n%s\n---\n%s", got, want.String())
	}
	if !strings.Contains(got, `'=HYPERLINK`) {
		t.Error("a formula was not neutralised in CSV")
	}
	if strings.HasPrefix(write(t, "csv", tb, Options{NoHeader: true}), "alias,") {
		t.Error("NoHeader kept the header")
	}
	// No TSV field may start with a formula character, quoted or not.
	if tsv := write(t, "tsv", tb, Options{}); !strings.Contains(tsv, "'=HYPERLINK") ||
		regexp.MustCompile(`(^|\t)"?[=+\-@]`).MatchString(tsv) {
		t.Errorf("a formula was not neutralised in TSV:\n%s", tsv)
	}
}

// JSON, JSON Lines and the JS module keep types and column order.
func TestJSONKeepsTypesAndColumnOrder(t *testing.T) {
	out := write(t, "json", sample(), Options{})
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatal(err)
	}
	if rows[0]["port"] != float64(22) || rows[0]["vpn"] != false || rows[1]["mode"] != "true" {
		t.Errorf("types lost: %+v", rows[0])
	}
	if tags, ok := rows[1]["tags"].([]any); !ok || len(tags) != 0 {
		t.Errorf("an empty list is not []: %#v", rows[1]["tags"])
	}
	if strings.Index(out, `"alias"`) > strings.Index(out, `"port"`) {
		t.Error("keys are not in column order")
	}
	var shaped struct {
		Columns []string `json:"columns"`
		Rows    [][]any  `json:"rows"`
	}
	if err := json.Unmarshal([]byte(write(t, "json", sample(), Options{Shape: "table", Compact: true})), &shaped); err != nil ||
		len(shaped.Columns) != 6 || len(shaped.Rows) != 2 {
		t.Errorf("table shape = %+v, %v", shaped, err)
	}
	if lines := strings.Split(strings.TrimSpace(write(t, "jsonl", sample(), Options{})), "\n"); len(lines) != 2 {
		t.Errorf("jsonl lines = %d", len(lines))
	}
	js := write(t, "js", sample(), Options{})
	body := strings.TrimSuffix(strings.TrimPrefix(js, "export default "), ";\n")
	if !strings.HasPrefix(js, "export default [") || json.Unmarshal([]byte(body), &rows) != nil {
		t.Errorf("js is not an ES module exporting the array:\n%s", js)
	}
}

// YAML keeps a string that looks like a number or a bool a string.
func TestYAMLKeepsStringsStrings(t *testing.T) {
	out := write(t, "yaml", sample(), Options{})
	var rows []map[string]any
	if err := yaml.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatal(err)
	}
	if rows[0]["mode"] != "0600" || rows[1]["mode"] != "true" || rows[0]["port"] != 22 || rows[1]["vpn"] != true {
		t.Errorf("yaml types: %+v / %+v", rows[0], rows[1])
	}
	var shaped map[string]any
	if err := yaml.Unmarshal([]byte(write(t, "yaml", sample(), Options{Shape: "table"})), &shaped); err != nil || shaped["columns"] == nil {
		t.Errorf("yaml table shape: %v %v", shaped, err)
	}
}

// PHP: a quote or a backslash cannot end a string early.
func TestPHPEscapesQuotesAndBackslashes(t *testing.T) {
	out := write(t, "php", sample(), Options{})
	if !strings.HasPrefix(out, "<?php\n\nreturn [") || !strings.HasSuffix(out, "];\n") {
		t.Fatalf("not a PHP file returning an array:\n%s", out)
	}
	if !strings.Contains(out, `'o\'brien\\box'`) {
		t.Errorf("quote or backslash not escaped:\n%s", out)
	}
	if !strings.Contains(out, "'port' => 22") || !strings.Contains(out, "'vpn' => false") || !strings.Contains(out, "'tags' => []") {
		t.Errorf("types not kept:\n%s", out)
	}
	// Every single-quoted string is closed: strip them, and no quote is left.
	if rest := regexp.MustCompile(`'(?:[^'\\]|\\.)*'`).ReplaceAllString(out, ""); strings.Contains(rest, "'") {
		t.Errorf("an unbalanced quote:\n%s", rest)
	}
	// Compact puts the array on one line; the only other line break is the one
	// inside a value, which a single-quoted PHP string holds as it is.
	if c := write(t, "php", sample(), Options{Compact: true}); strings.Count(c, "\n") != 4 || !strings.Contains(c, "'a | b\nc ") {
		t.Errorf("compact PHP is not on one line:\n%s", c)
	}
}

// Markdown: a pipe or a line break in a value cannot add a cell or a row.
func TestMarkdownCannotBeBrokenByAValue(t *testing.T) {
	out := write(t, "markdown", sample(), Options{})
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("lines = %d, want header, rule and two rows:\n%s", len(lines), out)
	}
	cells := func(l string) int { return len(regexp.MustCompile(`(^|[^\\])\|`).FindAllString(l, -1)) - 1 }
	for _, l := range lines {
		if cells(l) != 6 {
			t.Errorf("%d cells in %q", cells(l), l)
		}
	}
}

// HTML: every value is escaped, and the page runs nothing.
func TestHTMLEscapesEveryValue(t *testing.T) {
	out := write(t, "html", sample(), Options{Title: "<b>hosts</b>"})
	if strings.Contains(out, "<script>") || strings.Contains(out, "<b>") {
		t.Errorf("markup got through:\n%s", out)
	}
	if !strings.Contains(out, "&lt;/td&gt;&lt;script&gt;") || !strings.Contains(out, `<th scope="col">alias</th>`) {
		t.Errorf("escaping or headings missing:\n%s", out)
	}
}
