package selection

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/core/inventory"
	"github.com/simtabi/authama/src/internal/core/manifest"
	"github.com/simtabi/authama/src/internal/services/dataset"
	"github.com/simtabi/authama/src/internal/services/exports"
	"github.com/simtabi/authama/src/internal/services/keysvc"
	"github.com/simtabi/authama/src/internal/services/query"
	"github.com/simtabi/authama/src/internal/services/tokensvc"
	"github.com/simtabi/authama/src/internal/util/csvout"
	"github.com/simtabi/authama/src/internal/util/paths"
)

// fixture is a home with three profiles: simtabi (a code, named servers and
// an older one), adelsaiq, and work (a git host), plus an npm token.
func fixture(t *testing.T) (paths.Paths, dataset.Options) {
	t.Helper()
	base := t.TempDir()
	p := paths.Paths{SSHDir: filepath.Join(base, ".ssh"), ConfigDir: filepath.Join(base, "cfg")}
	userHome := filepath.Join(base, "home")
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	b := make([]byte, 18)
	_, _ = rand.Read(b)
	write(p.Manifest(), `{"version":1,"defaults":{"key_type":"ed25519","naming":{"envs":["prd","stg"]}},"profiles":{
	  "simtabi":{"key_scope":"per_service","code":"simbi","category":"orgs","hosts":[
	    {"alias":"simbi-prd-web-00","hostname":"203.0.113.10","user":"ploi","tags":["web","prd"]},
	    {"alias":"simbi-prd-db-00","hostname":"203.0.113.11","user":"ploi","tags":["db","prd"]},
	    {"alias":"oribi-web","hostname":"203.0.113.12","user":"ploi"}]},
	  "adelsaiq":{"key_scope":"per_service","code":"dani","category":"clients","hosts":[
	    {"alias":"dani-stg-web-00","hostname":"203.0.113.20","user":"ploi","tags":["web","stg"]}]},
	  "work":{"key_scope":"per_service","category":"personal","key_comment":"{alias}","hosts":[
	    {"alias":"gh","hostname":"github.com","user":"git"}]}}}`)
	write(p.Inventory(), `{"version":1,"keys":{}}`)
	write(filepath.Join(userHome, ".npmrc"), "//registry.npmjs.org/:_authToken="+hex.EncodeToString(b)+"\n")
	env := map[string]string{"HOME": userHome, "USERPROFILE": userHome}
	return p, dataset.Options{Getenv: func(k string) string { return env[k] }, Cwd: base}
}

func build(t *testing.T) (*dataset.Dataset, paths.Paths, dataset.Options) {
	t.Helper()
	p, o := fixture(t)
	ds, err := dataset.Build(p, o)
	if err != nil {
		t.Fatal(err)
	}
	return ds, p, o
}

func csvOf(t *testing.T, tb csvout.Table) string {
	t.Helper()
	var buf bytes.Buffer
	if err := csvout.Write(&buf, tb); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func resultCSV(t *testing.T, r Result) string {
	t.Helper()
	tb := csvout.Table{Header: r.Columns}
	for _, row := range r.Rows {
		tb.Rows = append(tb.Rows, Cells(row))
	}
	return csvOf(t, tb)
}

// Each grain's default columns write exactly the CSV its list wrote before
// the pipeline, from the same home: the guard that moving the old URLs onto
// selections changes no byte.
func TestTheDefaultsReproduceTheListCSVsByteForByte(t *testing.T) {
	ds, p, o := build(t)
	m, err := manifest.Load(p.Manifest())
	if err != nil {
		t.Fatal(err)
	}
	inv, _ := inventory.Load(p.Inventory())
	groups, err := query.New(m, inv, p.Providers()).Groups("", "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	krows, err := keysvc.New(m, inv, p.SSHDir).Rows("")
	if err != nil {
		t.Fatal(err)
	}
	refs, _ := m.KeyRefs()
	count, names := map[string]int{}, map[string][]string{}
	for _, r := range refs {
		count[r.Profile]++
	}
	for _, r := range krows {
		names[r.Ref.Profile] = append(names[r.Ref.Profile], r.Ref.KeyName)
	}
	var profs []exports.Profile
	for _, n := range m.ProfileNames() {
		pr := m.Profiles[n]
		store := pr.KeyStore
		if store == "" {
			store = "disk"
		}
		profs = append(profs, exports.Profile{Name: n, Category: pr.Category, KeyScope: pr.KeyScope, KeyStore: store,
			KeyComment: pr.KeyComment, Hosts: len(pr.Hosts), Keys: count[n], KeyNames: names[n]})
	}
	before := map[string]csvout.Table{
		GrainHost:    exports.Hosts(groups),
		GrainKey:     exports.Keys(krows),
		GrainProfile: exports.Profiles(profs),
		GrainToken:   exports.Tokens(tokensvc.NewEnv(p, o.Getenv, nil, o.Cwd).Inventory("", false)),
	}
	if len(before) != len(Grains) {
		t.Fatalf("%d lists compared, %d grains", len(before), len(Grains))
	}
	for _, g := range Grains {
		r, err := Run(ds, Selection{Version: Version, Grain: g})
		if err != nil {
			t.Fatal(err)
		}
		want, got := csvOf(t, before[g]), resultCSV(t, r)
		if len(before[g].Rows) == 0 {
			t.Fatalf("%s: the fixture has no rows to compare", g)
		}
		if got != want {
			t.Errorf("%s differs from its list's CSV:\n--- before\n%s--- selection\n%s", g, want, got)
		}
	}
}

func run(t *testing.T, ds *dataset.Dataset, s Selection) Result {
	t.Helper()
	s.Version = Version
	r, err := Run(ds, s)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func column(r Result, name string) []string {
	i := -1
	for j, c := range r.Columns {
		if c == name {
			i = j
		}
	}
	var out []string
	for _, row := range r.Rows {
		out = append(out, Cell(row[i]))
	}
	return out
}

func stage(r Result, name string) int {
	for _, s := range r.Stages {
		if s.Name == name {
			return s.Rows
		}
	}
	return -1
}

func TestFacetsChooseRowsAndCountEachStep(t *testing.T) {
	ds, _, _ := build(t)
	r := run(t, ds, Selection{Grain: GrainHost, Where: Where{Profile: []string{"simtabi", "adelsaiq"}, Env: []string{"prd"}}})
	if got := strings.Join(column(r, "alias"), ","); got != "simbi-prd-web-00,simbi-prd-db-00" {
		t.Errorf("prd hosts of simtabi and adelsaiq = %s", got)
	}
	if stage(r, "all") != 5 || stage(r, "profile") != 4 || stage(r, "key") != 4 || stage(r, "facets") != 2 || stage(r, "text") != 2 {
		t.Errorf("stages = %+v", r.Stages)
	}
	r = run(t, ds, Selection{Grain: GrainHost, Where: Where{Role: []string{"web"}, Tag: []string{"stg"}}})
	if got := strings.Join(column(r, "alias"), ","); got != "dani-stg-web-00" {
		t.Errorf("web hosts tagged stg = %s", got)
	}
	r = run(t, ds, Selection{Grain: GrainHost, Where: Where{Exclude: "SIMBI"}})
	if len(r.Rows) != 3 {
		t.Errorf("excluding simbi, ignoring case, left %d rows", len(r.Rows))
	}
	r = run(t, ds, Selection{Grain: GrainToken, Where: Where{Target: []string{"npm"}}})
	if len(r.Rows) != 1 {
		t.Errorf("npm tokens = %d", len(r.Rows))
	}
}

// A key chosen keeps the profiles holding it, the hosts using it and the key
// itself; narrowing cuts a profile's key list and counts to the chosen keys.
func TestAKeyChoosesAcrossGrainsAndNarrows(t *testing.T) {
	ds, _, _ := build(t)
	var web string
	for _, h := range ds.Hosts {
		if h.Alias == "simbi-prd-web-00" {
			web = h.Key
		}
	}
	whole := run(t, ds, Selection{Grain: GrainProfile, Where: Where{Key: []string{web}}})
	if got := column(whole, "profile"); len(got) != 1 || got[0] != "simtabi" {
		t.Fatalf("profiles holding %s = %v", web, got)
	}
	if column(whole, "keys")[0] != "3" || column(whole, "hosts")[0] != "3" {
		t.Errorf("whole: keys %s hosts %s; want the profile's 3 and 3", column(whole, "keys")[0], column(whole, "hosts")[0])
	}
	narrow := run(t, ds, Selection{Grain: GrainProfile, Narrow: true, Where: Where{Key: []string{web}}})
	if column(narrow, "keys")[0] != "1" || column(narrow, "hosts")[0] != "1" || column(narrow, "key_names")[0] != web {
		t.Errorf("narrowed: keys %s hosts %s names %s", column(narrow, "keys")[0], column(narrow, "hosts")[0], column(narrow, "key_names")[0])
	}
	if hosts := run(t, ds, Selection{Grain: GrainHost, Where: Where{Key: []string{"simtabi/" + web}}}); len(hosts.Rows) != 1 {
		t.Errorf("hosts using the key by its ref = %d", len(hosts.Rows))
	}
	if keys := run(t, ds, Selection{Grain: GrainKey, Where: Where{Key: []string{web}}}); len(keys.Rows) != 1 {
		t.Errorf("the key itself = %d", len(keys.Rows))
	}
}

func TestColumnsFollowRelationsSortAndLimit(t *testing.T) {
	ds, _, _ := build(t)
	r := run(t, ds, Selection{Grain: GrainHost,
		Columns: []Column{{Field: "alias"}, {Field: "profile.code", As: "code"}, {Field: "env"}, {Field: "key.type"}},
		Sort:    []Sort{{Field: "alias", Desc: true}}, Limit: 2})
	if strings.Join(r.Columns, ",") != "alias,code,env,key.type" {
		t.Errorf("columns = %v", r.Columns)
	}
	if got := strings.Join(column(r, "alias"), ","); got != "simbi-prd-web-00,simbi-prd-db-00" {
		t.Errorf("sorted descending, first two = %s", got)
	}
	if column(r, "code")[0] != "simbi" || column(r, "env")[0] != "prd" || column(r, "key.type")[0] != "ed25519" {
		t.Errorf("row = %v", Cells(r.Rows[0]))
	}
	if _, err := Run(ds, Selection{Version: Version, Grain: GrainHost, Columns: []Column{{Field: "nope"}}}); err == nil {
		t.Error("an unknown field was accepted")
	}
	if _, err := Run(ds, Selection{Version: Version, Grain: "planet"}); err == nil {
		t.Error("an unknown grain was accepted")
	}
}

func TestASavedSelectionOfAnotherVersionIsRefused(t *testing.T) {
	if _, err := Parse([]byte(`{"version":2,"grain":"host"}`)); err == nil || !strings.Contains(err.Error(), "version 2") {
		t.Errorf("version 2: %v", err)
	}
	if _, err := Parse([]byte(`{"version":1,"grain":"host","wher":{}}`)); err == nil {
		t.Error("an unknown field was accepted")
	}
	s, err := Parse([]byte(`{"version":1,"grain":"host","where":{"env":["prd"]},"narrow":true}`))
	if err != nil || s.Grain != GrainHost || s.Where.Env[0] != "prd" || !s.Narrow {
		t.Errorf("Parse = %+v, %v", s, err)
	}
}
