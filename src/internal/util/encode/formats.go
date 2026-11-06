package encode

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"html"
	"io"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/simtabi/authama/src/internal/util/csvout"
)

func init() {
	register(Format{Name: "csv", Extension: "csv", MediaType: "text/csv; charset=utf-8", Label: "CSV", Write: writeCSV})
	register(Format{Name: "tsv", Extension: "tsv", MediaType: "text/tab-separated-values; charset=utf-8", Label: "TSV", Write: writeTSV})
	register(Format{Name: "json", Extension: "json", MediaType: "application/json; charset=utf-8", Label: "JSON", Write: writeJSON})
	register(Format{Name: "jsonl", Extension: "jsonl", MediaType: "application/x-ndjson; charset=utf-8", Label: "JSON Lines", Write: writeJSONL})
	register(Format{Name: "yaml", Extension: "yaml", MediaType: "application/yaml; charset=utf-8", Label: "YAML", Write: writeYAML})
	register(Format{Name: "php", Extension: "php", MediaType: "text/x-php; charset=utf-8", Label: "PHP array", Write: writePHP})
	register(Format{Name: "js", Extension: "js", MediaType: "text/javascript; charset=utf-8", Label: "JavaScript array", Write: writeJS})
	register(Format{Name: "markdown", Extension: "md", MediaType: "text/markdown; charset=utf-8", Label: "Markdown table", Write: writeMarkdown})
	register(Format{Name: "html", Extension: "html", MediaType: "text/html; charset=utf-8", Label: "HTML table", Write: writeHTML})
}

func texts(r []any, n int) []string {
	out := make([]string, n)
	for i := range out {
		if i < len(r) {
			out[i] = Text(r[i])
		}
	}
	return out
}

// writeCSV is csvout's writer, so a CSV from here is byte for byte the CSV
// the lists always wrote: formula-safe cells, a header first.
func writeCSV(w io.Writer, t Table, o Options) error {
	ct := csvout.Table{Header: t.Columns}
	for _, r := range t.Rows {
		ct.Rows = append(ct.Rows, texts(r, len(t.Columns)))
	}
	if !o.NoHeader {
		return csvout.Write(w, ct)
	}
	var buf bytes.Buffer
	if err := csvout.Write(&buf, ct); err != nil {
		return err
	}
	_, rest, _ := bytes.Cut(buf.Bytes(), []byte("\n"))
	_, err := w.Write(rest)
	return err
}

// writeTSV is tab-separated, with the same formula-safe cells as CSV.
func writeTSV(w io.Writer, t Table, o Options) error {
	cw := csv.NewWriter(w)
	cw.Comma = '\t'
	if !o.NoHeader {
		if err := cw.Write(t.Columns); err != nil {
			return err
		}
	}
	for _, r := range t.Rows {
		cells := texts(r, len(t.Columns))
		for i, c := range cells {
			cells[i] = csvout.Cell(c)
		}
		if err := cw.Write(cells); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// MarshalJSON writes a row as an object with its keys in column order.
func (o orderedRow) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		vb, err := json.Marshal(o.vals[i])
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// tableShape is the {columns, rows} form of a table.
type tableShape struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// data is t as the value a structured format writes: an array of objects,
// or {columns, rows} for Shape "table".
func data(t Table, o Options) any {
	if o.Shape == "table" {
		rows := make([][]any, 0, len(t.Rows))
		for _, r := range t.Rows {
			out := make([]any, len(t.Columns))
			for i := range out {
				if i < len(r) {
					out[i] = plain(r[i])
				} else {
					out[i] = ""
				}
			}
			rows = append(rows, out)
		}
		return tableShape{Columns: t.Columns, Rows: rows}
	}
	return objects(t)
}

func marshalJSON(t Table, o Options) ([]byte, error) {
	if o.Compact {
		return json.Marshal(data(t, o))
	}
	return json.MarshalIndent(data(t, o), "", "  ")
}

func writeJSON(w io.Writer, t Table, o Options) error {
	b, err := marshalJSON(t, o)
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}

func writeJSONL(w io.Writer, t Table, _ Options) error {
	for _, r := range objects(t) {
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		if _, err := w.Write(append(b, '\n')); err != nil {
			return err
		}
	}
	return nil
}

// writeYAML writes the same data as JSON, as YAML: a sequence of mappings
// with keys in column order.
func writeYAML(w io.Writer, t Table, o Options) error {
	node := yamlNode(data(t, o))
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(node); err != nil {
		return err
	}
	return enc.Close()
}

func yamlNode(v any) *yaml.Node {
	switch x := v.(type) {
	case tableShape:
		m := &yaml.Node{Kind: yaml.MappingNode}
		rows := &yaml.Node{Kind: yaml.SequenceNode}
		for _, r := range x.Rows {
			row := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
			for _, c := range r {
				row.Content = append(row.Content, yamlNode(c))
			}
			rows.Content = append(rows.Content, row)
		}
		m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "columns"}, yamlNode(x.Columns),
			&yaml.Node{Kind: yaml.ScalarNode, Value: "rows"}, rows)
		return m
	case []orderedRow:
		seq := &yaml.Node{Kind: yaml.SequenceNode}
		for _, r := range x {
			seq.Content = append(seq.Content, yamlNode(r))
		}
		return seq
	case orderedRow:
		m := &yaml.Node{Kind: yaml.MappingNode}
		for i, k := range x.keys {
			m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k}, yamlNode(x.vals[i]))
		}
		return m
	case []string:
		seq := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
		for _, s := range x {
			seq.Content = append(seq.Content, yamlNode(s))
		}
		return seq
	case string:
		// Tagged as a string, so "true", "0600" or "2026-11-06" stay strings.
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: x}
	default:
		var n yaml.Node
		_ = n.Encode(x)
		return &n
	}
}

// writeJS is an ES module whose default export is the data, written as JSON
// literals, so nothing in a value can end the array early.
func writeJS(w io.Writer, t Table, o Options) error {
	b, err := marshalJSON(t, o)
	if err != nil {
		return err
	}
	_, err = io.WriteString(w, "export default "+string(b)+";\n")
	return err
}

// writePHP is a PHP file returning the data as an array: single-quoted
// strings, with \ and ' escaped, the only two characters such a string reads.
func writePHP(w io.Writer, t Table, o Options) error {
	_, err := io.WriteString(w, "<?php\n\nreturn "+php(data(t, o), "", o.Compact)+";\n")
	return err
}

func phpString(s string) string {
	return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'"
}

// phpArray writes items as a PHP array: on one line when compact, else one
// item a line, indented, each with a trailing comma.
func phpArray(items []string, ind string, compact bool) string {
	if len(items) == 0 {
		return "[]"
	}
	if compact {
		return "[" + strings.Join(items, ", ") + "]"
	}
	var b strings.Builder
	b.WriteString("[\n")
	for _, it := range items {
		b.WriteString(ind + "    " + it + ",\n")
	}
	b.WriteString(ind + "]")
	return b.String()
}

func php(v any, ind string, compact bool) string {
	in := ind + "    "
	switch x := v.(type) {
	case []orderedRow:
		items := make([]string, len(x))
		for i, r := range x {
			items[i] = php(r, in, compact)
		}
		return phpArray(items, ind, compact)
	case orderedRow:
		items := make([]string, len(x.keys))
		for i, k := range x.keys {
			items[i] = phpString(k) + " => " + php(x.vals[i], in, compact)
		}
		return phpArray(items, ind, compact)
	case tableShape:
		rows := make([]string, len(x.Rows))
		for i, r := range x.Rows {
			rows[i] = php(r, in+"    ", compact)
		}
		return phpArray([]string{
			"'columns' => " + php(x.Columns, in, compact),
			"'rows' => " + phpArray(rows, in, compact),
		}, ind, compact)
	case []any:
		items := make([]string, len(x))
		for i, e := range x {
			items[i] = php(e, in, true)
		}
		return "[" + strings.Join(items, ", ") + "]"
	case []string:
		items := make([]string, len(x))
		for i, e := range x {
			items[i] = phpString(e)
		}
		return "[" + strings.Join(items, ", ") + "]"
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case nil:
		return "null"
	default:
		return phpString(Text(x))
	}
}

// writeMarkdown is a pipe table. A | in a cell is escaped and a line break
// becomes <br>, so a value cannot add a column or end the table.
func writeMarkdown(w io.Writer, t Table, o Options) error {
	esc := strings.NewReplacer(`\`, `\\`, "|", `\|`, "\r\n", "<br>", "\n", "<br>", "\r", "<br>")
	line := func(cells []string) string {
		for i, c := range cells {
			cells[i] = esc.Replace(c)
		}
		return "| " + strings.Join(cells, " | ") + " |\n"
	}
	var b strings.Builder
	head := append([]string(nil), t.Columns...)
	if o.NoHeader {
		// A Markdown table needs a header row; blank headings keep it a table.
		for i := range head {
			head[i] = " "
		}
	}
	b.WriteString(line(head))
	rule := make([]string, len(t.Columns))
	for i := range rule {
		rule[i] = "---"
	}
	b.WriteString("| " + strings.Join(rule, " | ") + " |\n")
	for _, r := range t.Rows {
		b.WriteString(line(texts(r, len(t.Columns))))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// writeHTML is a standalone page holding one table, every value escaped,
// with no script and no outside resource.
func writeHTML(w io.Writer, t Table, o Options) error {
	title := o.Title
	if title == "" {
		title = "authama export"
	}
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\">\n<head>\n<meta charset=\"utf-8\">\n<title>")
	b.WriteString(html.EscapeString(title))
	b.WriteString("</title>\n<style>table{border-collapse:collapse;font:14px system-ui,sans-serif}" +
		"th,td{border:1px solid #ccc;padding:4px 8px;text-align:left;vertical-align:top}th{background:#f4f4f4}</style>\n" +
		"</head>\n<body>\n<table>\n")
	if !o.NoHeader {
		b.WriteString("<thead><tr>")
		for _, c := range t.Columns {
			b.WriteString("<th scope=\"col\">" + html.EscapeString(c) + "</th>")
		}
		b.WriteString("</tr></thead>\n")
	}
	b.WriteString("<tbody>\n")
	for _, r := range t.Rows {
		b.WriteString("<tr>")
		for _, c := range texts(r, len(t.Columns)) {
			b.WriteString("<td>" + html.EscapeString(c) + "</td>")
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</tbody>\n</table>\n</body>\n</html>\n")
	_, err := io.WriteString(w, b.String())
	return err
}
