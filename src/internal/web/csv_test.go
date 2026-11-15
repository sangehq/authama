package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/util/testcred"
)

// Every list the pages show downloads as CSV, header first, and the tokens
// list never carries a secret.
func TestEveryListDownloadsAsCSV(t *testing.T) {
	s, _, _ := newTestServer(t)
	npmrc := filepath.Join(t.TempDir(), ".npmrc")
	secret := testcred.Secret("npm")
	if err := os.WriteFile(npmrc, []byte("//registry.npmjs.org/:_authToken="+secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NPM_CONFIG_USERCONFIG", npmrc)
	c := signIn(t, s)
	heads := map[string]string{
		"hosts.csv":       "profile,alias,hostname,provider,key,status,tags",
		"keys.csv":        "key,profile,type,fingerprint,created,expires,hosts,on_disk,state",
		"tokens.csv":      "tool,host,kind,description,user,secret_set,file",
		"history.csv":     "op_id,started_at,finished_at,command,args,result,source,user,machine,snapshot,subjects,events",
		"events.csv":      "time,seen,kind,area,path,size,mode,detail",
		"profiles.csv":    "profile,category,key_scope,key_store,hosts,keys,key_comment,key_names",
		"snapshots.csv":   "snapshot,taken,bytes,holds_private_keys",
		"deployments.csv": "key_path,fingerprint,status,target,method,verified,date",
	}
	if len(heads) != len(csvLists) {
		t.Fatalf("%d lists tested, %d served", len(heads), len(csvLists))
	}
	for name, head := range heads {
		rec := c.get("/exports/" + name)
		body := rec.Body.String()
		if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/csv") {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Header().Get("Content-Type"))
			continue
		}
		if !strings.HasPrefix(body, head+"\n") {
			t.Errorf("%s starts %q, want %q", name, strings.SplitN(body, "\n", 2)[0], head)
		}
		if !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") {
			t.Errorf("%s is not an attachment", name)
		}
		if strings.Contains(body, secret) {
			t.Errorf("%s holds a secret", name)
		}
	}
	if body := c.get("/exports/tokens.csv").Body.String(); !strings.Contains(body, "\nnpm,registry.npmjs.org,") || !strings.Contains(body, ",true,"+npmrc) {
		t.Errorf("tokens.csv does not list the npm token as set:\n%s", body)
	}
}

// A list's CSV is what its page shows: the history and events filters in the
// query narrow the download the same way.
func TestCSVKeepsThePagesFilters(t *testing.T) {
	s, _, _ := newTestServer(t)
	c := signIn(t, s)
	f := historyFilter(map[string][]string{tokenFilterParam: {"npm"}, "command": {"token"}, "since": {"2026-01-01"}})
	if f.Token != "npm" || f.Command != "token" || f.Since != "2026-01-01" {
		t.Fatalf("historyFilter dropped a filter: %+v", f)
	}
	if ef := eventsFilter(map[string][]string{"area": {"auth"}, "kind": {"bogus"}}); ef.Area != "auth" || ef.Kind != "" {
		t.Fatalf("eventsFilter = %+v, want area auth and no kind", ef)
	}
	all := c.get("/exports/history.csv").Body.String()
	none := c.get("/exports/history.csv?command=no-such-command").Body.String()
	if strings.Count(none, "\n") != 1 {
		t.Errorf("a filter matching nothing still exported rows:\n%s", none)
	}
	if strings.Count(all, "\n") < strings.Count(none, "\n") {
		t.Errorf("the unfiltered export is smaller than the filtered one")
	}
}
