package web

import (
	"github.com/simtabi/authama/src/internal/services/editor"

	"encoding/csv"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// A request from the side panel gets the page's content alone: no layout, no
// navigation, the title on the frame. The same URL without it is a full page.
func TestTheSidePanelGetsAPagesContentAlone(t *testing.T) {
	s, _, opID := newTestServer(t)
	c := signIn(t, s)
	r, _ := http.NewRequest(http.MethodGet, "http://"+testHost+"/history/"+opID, nil)
	r.AddCookie(c.cookie)
	r.Header.Set(panelHeader, "1")
	rec := do(s, r)
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.HasPrefix(body, "<div data-panel-page") || strings.Contains(body, "<html") || strings.Contains(body, "navbar") {
		t.Fatalf("panel answer: %d\n%.300s", rec.Code, body)
	}
	if !strings.Contains(rec.Header().Get("Vary"), panelHeader) {
		t.Error("no Vary on the panel header: a cache could serve one answer for the other")
	}
	full := c.get("/history/" + opID).Body.String()
	if !strings.Contains(full, "<html") || !strings.Contains(full, `data-side-panel`) || !strings.Contains(full, `data-csv-dialog`) {
		t.Fatal("the full page lacks the layout, the side panel shell or the CSV dialog")
	}
}

// A list's CSV leaves out the columns and rows asked for, and its preview is
// JSON describing exactly that file.
func TestACSVCanLeaveOutColumnsAndRows(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	all := readCSV(t, c.get("/exports/hosts.csv").Body.String())
	if len(all) < 2 {
		t.Fatalf("hosts.csv: %v", all)
	}
	header := all[0]
	drop := header[len(header)-1]
	shaped := readCSV(t, c.get("/exports/hosts.csv?drop="+drop+"&exclude=github").Body.String())
	if strings.Join(shaped[0], ",") != strings.Join(header[:len(header)-1], ",") {
		t.Errorf("drop=%s: header %v", drop, shaped[0])
	}
	for _, row := range shaped[1:] {
		if strings.Contains(strings.ToLower(strings.Join(row, ",")), "github") {
			t.Errorf("exclude=github kept %v", row)
		}
	}
	rec := c.get("/exports/hosts.csv?preview=1&drop=" + drop)
	var p csvPreview
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("preview: %v %s", err, rec.Body.String())
	}
	if strings.Join(p.Columns, ",") != strings.Join(header, ",") || len(p.Header) != len(header)-1 || p.Total != len(all)-1 || p.Kept != p.Total {
		t.Errorf("preview %+v against %v", p, header)
	}
}

// A list with a profile column can be cut to some profiles: the file holds
// only their rows, even with that column dropped, the preview lists every
// profile with its row count, and a list with no profile column lists none.
func TestACSVCanHoldSomeProfilesOnly(t *testing.T) {
	s, home, _ := newTestServer(t)
	ed := editor.New(home)
	if err := ed.AddProfile("acme", "per_service", nil); err != nil {
		t.Fatal(err)
	}
	host, user := "203.0.113.30", "deploy"
	for _, alias := range []string{"acme-prd-web-00", "acme-prd-db-00"} {
		if err := ed.AddHost("acme", alias, editor.HostFields{Hostname: &host, User: &user}); err != nil {
			t.Fatal(err)
		}
	}
	c := signIn(t, s)
	all := readCSV(t, c.get("/exports/hosts.csv").Body.String())
	col := -1
	for i, h := range all[0] {
		if h == "profile" {
			col = i
		}
	}
	if col < 0 {
		t.Fatalf("hosts.csv has no profile column: %v", all[0])
	}
	counts := map[string]int{}
	for _, r := range all[1:] {
		counts[r[col]]++
	}
	if len(counts) < 2 {
		t.Fatalf("the fixture needs two profiles with hosts, has %v", counts)
	}
	var one string
	for name := range counts {
		one = name
		break
	}
	only := readCSV(t, c.get("/exports/hosts.csv?profile="+one+"&drop=profile").Body.String())
	if len(only)-1 != counts[one] {
		t.Fatalf("profile=%s: %d rows, want %d", one, len(only)-1, counts[one])
	}
	for _, h := range only[0] {
		if h == "profile" {
			t.Fatal("drop=profile kept the column")
		}
	}

	var p csvPreview
	if err := json.Unmarshal(c.get("/exports/hosts.csv?preview=1&profile="+one).Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, pr := range p.Profiles {
		got[pr.Name] = pr.Rows
	}
	if len(got) != len(counts) || p.Kept != counts[one] || p.Total != len(all)-1 {
		t.Errorf("preview profiles %v kept %d of %d; want %v, %d", got, p.Kept, p.Total, counts, counts[one])
	}
	for name, n := range counts {
		if got[name] != n {
			t.Errorf("preview: %s has %d rows, want %d", name, got[name], n)
		}
	}

	var tp csvPreview
	_ = json.Unmarshal(c.get("/exports/tokens.csv?preview=1").Body.Bytes(), &tp)
	if tp.Profiles != nil {
		t.Errorf("tokens.csv has no profile column but listed %v", tp.Profiles)
	}
}

func readCSV(t *testing.T, s string) [][]string {
	t.Helper()
	r := csv.NewReader(strings.NewReader(s))
	r.FieldsPerRecord = -1
	out, err := r.ReadAll()
	if err != nil {
		t.Fatalf("not CSV: %v\n%s", err, s)
	}
	return out
}
