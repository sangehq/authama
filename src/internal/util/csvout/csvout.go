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
