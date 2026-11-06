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
//	?key=simtabi_web-ed25519                 only those keys' rows
//	?drop=user,port&exclude=staging          the file without those
//	?drop=user&preview=1                     JSON: the first rows, and counts
type csvShape struct {
	Profiles []string
	Keys     []string
	Drop     []string
	Exclude  string
	Preview  bool
}

func shapeOf(q url.Values) csvShape {
	s := csvShape{Exclude: strings.TrimSpace(q.Get("exclude")), Preview: q.Get("preview") == "1"}
	s.Drop = splitList(q.Get("drop"))
	s.Profiles = splitList(q.Get("profile"))
	s.Keys = splitList(q.Get("key"))
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
	return len(s.Drop) > 0 || len(s.Profiles) > 0 || len(s.Keys) > 0 || s.Exclude != "" || s.Preview
}

// profileColumns are the names a list's profile column goes by: profile in
// authama's own lists, Groups in the Termius import.
var profileColumns = []string{"profile", "Groups"}

// keyColumns are the names a list's key column goes by.
var keyColumns = []string{"key"}

// stages is t after each filter in turn: profiles, then keys, then the
// excluded text, before any column is dropped, so a dropped column can still
// choose rows, and the preview can say which filter left a row out.
func (s csvShape) stages(t csvout.Table) (inProfiles, inKeys, kept csvout.Table) {
	inProfiles = t.Only(t.Column(profileColumns...), s.Profiles)
	inKeys = inProfiles.Only(t.Column(keyColumns...), s.Keys)
	kept = inKeys.Without(s.Exclude)
	return inProfiles, inKeys, kept
}

// apply shapes t.
func (s csvShape) apply(t csvout.Table) csvout.Table {
	_, _, kept := s.stages(t)
	return kept.Drop(s.Drop...)
}

// csvProfile is one profile a list's rows belong to, and how many it has.
type csvProfile struct {
	Name string `json:"name"`
	Rows int    `json:"rows"`
}

// csvKey is one key a list's rows name, the profile it belongs to ("" when
// the list has no profile column), and how many rows name it.
type csvKey struct {
	Name    string `json:"name"`
	Profile string `json:"profile,omitempty"`
	Rows    int    `json:"rows"`
}

// keysOf lists the keys t's rows name, in first-seen order, or nil when t has
// no key column. A row with no key (a host whose key is not set) is not one.
func keysOf(t csvout.Table) []csvKey {
	kc := t.Column(keyColumns...)
	if kc < 0 {
		return nil
	}
	pc := t.Column(profileColumns...)
	out := []csvKey{}
	at := map[string]int{}
	for _, r := range t.Rows {
		if kc >= len(r) || r[kc] == "" {
			continue
		}
		prof := ""
		if pc >= 0 && pc < len(r) {
			prof = r[pc]
		}
		id := prof + "\x00" + r[kc]
		i, ok := at[id]
		if !ok {
			i = len(out)
			at[id] = i
			out = append(out, csvKey{Name: r[kc], Profile: prof})
		}
		out[i].Rows++
	}
	return out
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
	// Keys is every key the rows name, with its profile, for a list with a
	// key column, so the page can offer the keys of the profiles chosen.
	Keys    []csvKey   `json:"keys,omitempty"`
	Columns []string   `json:"columns"`
	Header  []string   `json:"header"`
	Rows    [][]string `json:"rows"`
	Total   int        `json:"total"` // every row
	// InProfiles and InKeys are the rows left after the profile filter and
	// then the key filter, so the page can say which filter left rows out.
	InProfiles int `json:"in_profiles"`
	InKeys     int `json:"in_keys"`
	Kept       int `json:"kept"` // after the excluded text too
}

// writeCSV answers with t, shaped by the request: the file, or its preview.
func (h pages) writeCSV(w http.ResponseWriter, r *http.Request, filename string, t csvout.Table) {
	shape := shapeOf(r.URL.Query())
	shaped := shape.apply(t)
	w.Header().Set("Cache-Control", "no-store")
	if shape.Preview {
		inProfiles, inKeys, _ := shape.stages(t)
		head := shaped.Head(PreviewRows)
		p := csvPreview{Profiles: profilesOf(t), Keys: keysOf(t), Columns: t.Header, Header: head.Header, Rows: head.Rows,
			Total: len(t.Rows), InProfiles: len(inProfiles.Rows), InKeys: len(inKeys.Rows), Kept: len(shaped.Rows)}
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
