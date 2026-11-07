// Package encode writes a table of typed rows in a file format: the last
// stage of the export pipeline (dataset, selection, format).
//
// Every format escapes for itself, because each is read by something
// different: a spreadsheet runs a cell starting with "=", a browser runs a
// "</table><script>", PHP ends a string at an unescaped quote, a Markdown
// table splits on "|". Each format has a test for its own hazard.
package encode

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// Table is what a format writes: column headings, and rows of strings, ints,
// int64s, bools and lists of strings, in column order.
type Table struct {
	Columns []string
	Rows    [][]any
}

// Options tune a format. A format ignores what does not apply to it.
type Options struct {
	NoHeader bool   // csv, tsv, markdown, html: leave the heading row out
	Compact  bool   // json, js, php, yaml: no indentation
	Shape    string // json, js, php, yaml: "objects" (default) or "table" ({columns, rows})
	Title    string // html: the page title
	// IncludeGit keeps git hosts (user git, or a github or gitlab provider)
	// in a host format, which otherwise leaves them out: they are not shells.
	IncludeGit bool
}

// Format is one file format.
type Format struct {
	Name      string // csv, json, ...
	Extension string // without the dot
	MediaType string
	Label     string // for a page: "CSV", "PHP array", ...
	Write     func(w io.Writer, t Table, o Options) error
	// Hosts is set on a host format (hosts.go): it writes the hosts a
	// selection kept, read from a HostSource, and Write refuses a table.
	Hosts func(src HostSource, aliases []string, o Options) (*ClientExport, error)
	// Client is the client format's own name in Clients, which the pipeline
	// may call otherwise: the generic "csv" client is "hosts-csv" there, since
	// "csv" is the table format.
	Client string
}

var registry = map[string]Format{}

func register(f Format) { registry[f.Name] = f }

// Get is the format of that name.
func Get(name string) (Format, error) {
	f, ok := registry[strings.ToLower(name)]
	if !ok {
		return Format{}, fmt.Errorf("%w %q; one of %s", ErrUnknownFormat, name, strings.Join(Names(), ", "))
	}
	return f, nil
}

// Names lists the formats, sorted.
func Names() []string {
	out := make([]string, 0, len(registry))
	for n := range registry {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// ErrUnknownFormat is a format this package does not have.
var ErrUnknownFormat = errors.New("encode: unknown format")

// Text is a value as text: lists joined by spaces, as the CSVs always wrote them.
func Text(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case bool:
		return strconv.FormatBool(x)
	case []string:
		return strings.Join(x, " ")
	default:
		return fmt.Sprint(x)
	}
}

// plain is a value as JSON-like data: lists stay lists, never nil, so an
// empty list is [] and not null.
func plain(v any) any {
	switch x := v.(type) {
	case nil:
		return ""
	case []string:
		if x == nil {
			return []string{}
		}
		return x
	default:
		return v
	}
}

// objects is t as one ordered map per row.
func objects(t Table) []orderedRow {
	out := make([]orderedRow, 0, len(t.Rows))
	for _, r := range t.Rows {
		o := orderedRow{keys: t.Columns, vals: make([]any, len(t.Columns))}
		for i := range t.Columns {
			if i < len(r) {
				o.vals[i] = plain(r[i])
			} else {
				o.vals[i] = ""
			}
		}
		out = append(out, o)
	}
	return out
}

// orderedRow is a row whose keys keep column order when written as an object.
type orderedRow struct {
	keys []string
	vals []any
}
