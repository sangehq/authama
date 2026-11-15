package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/sangehq/authama/src/internal/util/csvout"
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
//	?key=simtabi_web-ed25519&narrow=1        and, in a list of profiles, their
//	                                         keys and hosts counted for those keys
//	?drop=user,port&exclude=staging          the file without those
//	?drop=user&preview=1                     JSON: the first rows, and counts
type csvShape struct {
	Profiles []string
	Keys     []string
	Drop     []string
	Exclude  string
	Preview  bool
	// Narrow, for a list whose rows hold a list of keys (profiles.csv's
	// key_names), cuts each row's keys down to the ones chosen and counts its
	// keys and hosts for those alone; without it a row is kept or left out
	// whole.
	Narrow bool
	// keyHosts is how many hosts use each key, for counting hosts when
	// narrowing; nil for a list that has no such count.
	keyHosts map[string]int
}

func shapeOf(q url.Values) csvShape {
	s := csvShape{Exclude: strings.TrimSpace(q.Get("exclude")), Preview: q.Get("preview") == "1", Narrow: q.Get("narrow") == "1"}
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
	return len(s.Drop) > 0 || len(s.Profiles) > 0 || len(s.Keys) > 0 || s.Exclude != "" || s.Preview || s.Narrow
}

// profileColumns are the names a list's profile column goes by: profile in
// authama's own lists, Groups in the Termius import.
var profileColumns = []string{"profile", "Groups"}

// keyColumns are the names a list's key column goes by: one key per row.
var keyColumns = []string{"key"}

// keyListColumns are the names of a column holding a row's keys, separated
// by spaces: profiles.csv's key_names, one row per profile.
var keyListColumns = []string{"key_names"}

// keysIn is a key-list cell's keys.
func keysIn(cell string) []string { return strings.Fields(cell) }

// byKeys keeps t's rows that name one of the keys chosen: by its key column,
// or by its key-list column, where Narrow also cuts the row down to those
// keys. With no key chosen, or no key column, t is unchanged.
func (s csvShape) byKeys(t csvout.Table) csvout.Table {
	if len(s.Keys) == 0 {
		return t
	}
	if kc := t.Column(keyColumns...); kc >= 0 {
		return t.Only(kc, s.Keys)
	}
	lc := t.Column(keyListColumns...)
	if lc < 0 {
		return t
	}
	want := map[string]bool{}
	for _, k := range s.Keys {
		want[k] = true
	}
	keysCol, hostsCol := t.Column("keys"), t.Column("hosts")
	out := csvout.Table{Header: t.Header}
	for _, r := range t.Rows {
		if lc >= len(r) {
			continue
		}
		var kept []string
		for _, k := range keysIn(r[lc]) {
			if want[k] {
				kept = append(kept, k)
			}
		}
		if len(kept) == 0 {
			continue
		}
		if s.Narrow {
			r = append([]string(nil), r...)
			r[lc] = strings.Join(kept, " ")
			if keysCol >= 0 && keysCol < len(r) {
				r[keysCol] = strconv.Itoa(len(kept))
			}
			if hostsCol >= 0 && hostsCol < len(r) && s.keyHosts != nil {
				n := 0
				for _, k := range kept {
					n += s.keyHosts[k]
				}
				r[hostsCol] = strconv.Itoa(n)
			}
		}
		out.Rows = append(out.Rows, r)
	}
	return out
}

// stages is t after each filter in turn: profiles, then keys, then the
// excluded text, before any column is dropped, so a dropped column can still
// choose rows, and the preview can say which filter left a row out.
func (s csvShape) stages(t csvout.Table) (inProfiles, inKeys, kept csvout.Table) {
	inProfiles = t.Only(t.Column(profileColumns...), s.Profiles)
	inKeys = s.byKeys(inProfiles)
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
// the list has no profile column), how many rows name it, and, for a list
// of profiles, how many hosts use it.
type csvKey struct {
	Name    string `json:"name"`
	Profile string `json:"profile,omitempty"`
	Rows    int    `json:"rows"`
	Hosts   *int   `json:"hosts,omitempty"`
}

// keysOf lists the keys t's rows name, in first-seen order, or nil when t has
// neither a key column nor a key-list column. A row with no key (a host
// whose key is not set) names none.
func keysOf(t csvout.Table, keyHosts map[string]int) []csvKey {
	kc := t.Column(keyColumns...)
	lc := t.Column(keyListColumns...)
	if kc < 0 && lc < 0 {
		return nil
	}
	pc := t.Column(profileColumns...)
	out := []csvKey{}
	at := map[string]int{}
	for _, r := range t.Rows {
		var names []string
		switch {
		case kc >= 0 && kc < len(r) && r[kc] != "":
			names = []string{r[kc]}
		case kc < 0 && lc < len(r):
			names = keysIn(r[lc])
		}
		prof := ""
		if pc >= 0 && pc < len(r) {
			prof = r[pc]
		}
		for _, name := range names {
			id := prof + "\x00" + name
			i, ok := at[id]
			if !ok {
				i = len(out)
				at[id] = i
				out = append(out, csvKey{Name: name, Profile: prof})
				if kc < 0 && keyHosts != nil {
					n := keyHosts[name]
					out[i].Hosts = &n
				}
			}
			out[i].Rows++
		}
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
	Keys []csvKey `json:"keys,omitempty"`
	// KeyList is a list whose rows hold several keys each (profiles.csv), so
	// the page can offer to narrow them to the keys chosen.
	KeyList bool       `json:"key_list,omitempty"`
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
// keyHosts is how many hosts use each key, for a list of profiles; nil for
// any other.
func (h pages) writeCSV(w http.ResponseWriter, r *http.Request, filename string, t csvout.Table, keyHosts map[string]int) {
	shape := shapeOf(r.URL.Query())
	shape.keyHosts = keyHosts
	shaped := shape.apply(t)
	w.Header().Set("Cache-Control", "no-store")
	if shape.Preview {
		inProfiles, inKeys, _ := shape.stages(t)
		head := shaped.Head(PreviewRows)
		p := csvPreview{Profiles: profilesOf(t), Keys: keysOf(t, keyHosts), Columns: t.Header,
			KeyList: t.Column(keyColumns...) < 0 && t.Column(keyListColumns...) >= 0, Header: head.Header, Rows: head.Rows,
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
