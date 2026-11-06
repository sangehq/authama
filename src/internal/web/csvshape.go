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

// csvShape is how a CSV download was asked to look: the profiles whose rows
// it holds, columns left out, rows containing some text left out, and
// whether to answer with a preview instead of the file. Every list's CSV and
// the Termius export take it:
//
//	?profile=simtabi,adelsaiq                only those profiles' rows
//	?drop=user,port&exclude=staging          the file without those
//	?drop=user&preview=1                     JSON: the first rows, and counts
type csvShape struct {
	Profiles []string
	Drop     []string
	Exclude  string
	Preview  bool
}

func shapeOf(q url.Values) csvShape {
	s := csvShape{Exclude: strings.TrimSpace(q.Get("exclude")), Preview: q.Get("preview") == "1"}
	s.Drop = splitList(q.Get("drop"))
	s.Profiles = splitList(q.Get("profile"))
	return s
}

func splitList(v string) []string {
	var out []string
	for _, d := range strings.Split(v, ",") {
		if d = strings.TrimSpace(d); d != "" {
			out = append(out, d)
		}
	}
	return out
}

func (s csvShape) any() bool {
	return len(s.Drop) > 0 || len(s.Profiles) > 0 || s.Exclude != "" || s.Preview
}

// profileColumns are the names a list's profile column goes by: profile in
// authama's own lists, Groups in the Termius import.
var profileColumns = []string{"profile", "Groups"}

// apply shapes t. The profile filter runs first, so a column dropped from
// the file can still choose its rows.
func (s csvShape) apply(t csvout.Table) csvout.Table {
	return t.Only(t.Column(profileColumns...), s.Profiles).Without(s.Exclude).Drop(s.Drop...)
}

// csvProfile is one profile a list's rows belong to, and how many it has.
type csvProfile struct {
	Name string `json:"name"`
	Rows int    `json:"rows"`
}

// profilesOf lists the profiles t's rows belong to, in first-seen order, or
// nil when t has no profile column.
func profilesOf(t csvout.Table) []csvProfile {
	col := t.Column(profileColumns...)
	if col < 0 {
		return nil
	}
	out := []csvProfile{}
	at := map[string]int{}
	for _, r := range t.Rows {
		if col >= len(r) {
			continue
		}
		i, ok := at[r[col]]
		if !ok {
			i = len(out)
			at[r[col]] = i
			out = append(out, csvProfile{Name: r[col]})
		}
		out[i].Rows++
	}
	return out
}

// csvPreview is the answer to ?preview=1. Columns is every column the file
// can have, so the page can offer the ones dropped too.
type csvPreview struct {
	// Profiles is every profile the rows belong to, chosen or not, so the
	// page can offer them; absent for a list with no profile column.
	Profiles []csvProfile `json:"profiles,omitempty"`
	Columns  []string     `json:"columns"`
	Header   []string     `json:"header"`
	Rows     [][]string   `json:"rows"`
	Total    int          `json:"total"` // rows before Exclude
	Kept     int          `json:"kept"`  // rows after it
}

// writeCSV answers with t, shaped by the request: the file, or its preview.
func (h pages) writeCSV(w http.ResponseWriter, r *http.Request, filename string, t csvout.Table) {
	shape := shapeOf(r.URL.Query())
	shaped := shape.apply(t)
	w.Header().Set("Cache-Control", "no-store")
	if shape.Preview {
		head := shaped.Head(PreviewRows)
		p := csvPreview{Profiles: profilesOf(t), Columns: t.Header, Header: head.Header, Rows: head.Rows,
			Total: len(t.Rows), Kept: len(shaped.Rows)}
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
