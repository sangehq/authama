// Package csvout writes tables as CSV (RFC 4180): a header row, then one row
// per item, quoted where a cell needs it.
//
// Every cell is neutralised against CSV injection first. A spreadsheet reads
// a cell that starts with =, +, -, @, a tab or a carriage return as a
// formula, so a host alias or a file name chosen by someone else could run
// one when the export is opened. Such a cell is written with a leading
// apostrophe, which spreadsheets show as text and strip.
package csvout

import (
	"encoding/csv"
	"io"
	"strings"
)

// Table is a header and its rows.
type Table struct {
	Header []string
	Rows   [][]string
}

// Write writes t to w as CSV.
func Write(w io.Writer, t Table) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(t.Header); err != nil {
		return err
	}
	for _, r := range t.Rows {
		safe := make([]string, len(r))
		for i, c := range r {
			safe[i] = Cell(c)
		}
		if err := cw.Write(safe); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// Cell is one value made safe to open in a spreadsheet.
func Cell(v string) string {
	if v != "" && strings.ContainsRune("=+-@\t\r", rune(v[0])) {
		return "'" + v
	}
	return v
}

// Drop returns t without the named columns. A name that is not a column is
// ignored, so a saved choice survives a column being renamed or removed.
func (t Table) Drop(names ...string) Table {
	if len(names) == 0 {
		return t
	}
	gone := map[string]bool{}
	for _, n := range names {
		gone[strings.TrimSpace(n)] = true
	}
	var keep []int
	out := Table{}
	for i, h := range t.Header {
		if !gone[h] {
			keep = append(keep, i)
			out.Header = append(out.Header, h)
		}
	}
	for _, r := range t.Rows {
		row := make([]string, 0, len(keep))
		for _, i := range keep {
			if i < len(r) {
				row = append(row, r[i])
			} else {
				row = append(row, "")
			}
		}
		out.Rows = append(out.Rows, row)
	}
	return out
}

// Without returns t without the rows in which any cell contains text,
// ignoring case. An empty text keeps every row.
func (t Table) Without(text string) Table {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return t
	}
	out := Table{Header: t.Header}
	for _, r := range t.Rows {
		hit := false
		for _, c := range r {
			if strings.Contains(strings.ToLower(c), text) {
				hit = true
				break
			}
		}
		if !hit {
			out.Rows = append(out.Rows, r)
		}
	}
	return out
}

// Head returns t with at most n rows.
// Column is the index of the first column named one of names, or -1.
func (t Table) Column(names ...string) int {
	for i, h := range t.Header {
		for _, n := range names {
			if h == n {
				return i
			}
		}
	}
	return -1
}

// Only keeps the rows whose column col holds one of values; with no values,
// or no such column, it keeps every row.
func (t Table) Only(col int, values []string) Table {
	if col < 0 || len(values) == 0 {
		return t
	}
	keep := map[string]bool{}
	for _, v := range values {
		keep[v] = true
	}
	out := Table{Header: t.Header}
	for _, r := range t.Rows {
		if col < len(r) && keep[r[col]] {
			out.Rows = append(out.Rows, r)
		}
	}
	return out
}

func (t Table) Head(n int) Table {
	if n >= 0 && len(t.Rows) > n {
		t.Rows = t.Rows[:n]
	}
	return t
}

// Parse reads CSV text with a header row into a Table, so a file produced
// elsewhere (the Termius export) can be filtered the same way.
func Parse(text string) (Table, error) {
	r := csv.NewReader(strings.NewReader(text))
	r.FieldsPerRecord = -1
	recs, err := r.ReadAll()
	if err != nil || len(recs) == 0 {
		return Table{}, err
	}
	return Table{Header: recs[0], Rows: recs[1:]}, nil
}
