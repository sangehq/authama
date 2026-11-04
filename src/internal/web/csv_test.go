package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/util/testcred"
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
		"hosts.csv":   "profile,alias,hostname,provider,key,status,tags",
		"keys.csv":    "key,profile,type,fingerprint,created,expires,hosts,on_disk,state",
		"tokens.csv":  "tool,host,kind,description,user,secret_set,file",
		"history.csv": "op_id,started_at,finished_at,command,args,result,source,user,machine,snapshot",
		"events.csv":  "time,seen,kind,area,path,size,mode,detail",
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
