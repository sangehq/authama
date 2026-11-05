package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/simtabi/authama/src/internal/util/csvout"
)

// PreviewRows is how many rows a CSV preview shows.
const PreviewRows = 25

// csvShape is how a CSV download was asked to look: columns left out, rows
// containing some text left out, and whether to answer with a preview
// instead of the file. Every list's CSV and the Termius export take it:
//
//	?drop=user,port&exclude=staging          the file without those
//	?drop=user&preview=1                     JSON: the first rows, and counts
type csvShape struct {
	Drop    []string
	Exclude string
	Preview bool
}

func shapeOf(q url.Values) csvShape {
	s := csvShape{Exclude: strings.TrimSpace(q.Get("exclude")), Preview: q.Get("preview") == "1"}
	for _, d := range strings.Split(q.Get("drop"), ",") {
		if d = strings.TrimSpace(d); d != "" {
			s.Drop = append(s.Drop, d)
		}
	}
	return s
}

func (s csvShape) any() bool { return len(s.Drop) > 0 || s.Exclude != "" || s.Preview }

// apply shapes t.
func (s csvShape) apply(t csvout.Table) csvout.Table {
	return t.Without(s.Exclude).Drop(s.Drop...)
}

// csvPreview is the answer to ?preview=1. Columns is every column the file
// can have, so the page can offer the ones dropped too.
type csvPreview struct {
	Columns []string   `json:"columns"`
	Header  []string   `json:"header"`
	Rows    [][]string `json:"rows"`
	Total   int        `json:"total"` // rows before Exclude
	Kept    int        `json:"kept"`  // rows after it
}

// writeCSV answers with t, shaped by the request: the file, or its preview.
func (h pages) writeCSV(w http.ResponseWriter, r *http.Request, filename string, t csvout.Table) {
	shape := shapeOf(r.URL.Query())
	shaped := shape.apply(t)
	w.Header().Set("Cache-Control", "no-store")
	if shape.Preview {
		head := shaped.Head(PreviewRows)
		p := csvPreview{Columns: t.Header, Header: head.Header, Rows: head.Rows, Total: len(t.Rows), Kept: len(shaped.Rows)}
		if p.Rows == nil {
			p.Rows = [][]string{}
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(p)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	_ = csvout.Write(w, shaped)
}
