package knownhosts

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/core/manifest"
)

// InitEach reports each host before it is scanned and once more when all are
// done, so a job page can count them: the web's "Pin host keys" job shows a
// percentage and the time left from these calls.
func TestInitEachCountsTheHostsItScans(t *testing.T) {
	fakeKeyscan(t, str(""))
	const mj = `{"version":1,"defaults":{"key_type":"ed25519"},"profiles":{
	  "work":{"key_scope":"per_service","hosts":[
	    {"alias":"a","hostname":"a.example","user":"u"},
	    {"alias":"b","hostname":"b.example","user":"u"},
	    {"alias":"c","hostname":"c.example","user":"u"}
	  ]},
	  "home":{"key_scope":"per_service","hosts":[{"alias":"d","hostname":"d.example","user":"u"}]}}}`
	var m manifest.Manifest
	if err := json.Unmarshal([]byte(mj), &m); err != nil {
		t.Fatal(err)
	}
	var calls []string
	rep, err := New(t.TempDir()).InitEach(&m, "work", false, false, func(done, total int, alias string) {
		calls = append(calls, fmt.Sprintf("%d/%d %s", done, total, alias))
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "0/3 a|1/3 b|2/3 c|3/3 "; strings.Join(calls, "|") != want {
		t.Errorf("progress calls = %q, want %q", strings.Join(calls, "|"), want)
	}
	if len(rep.Results) != 3 {
		t.Errorf("scanned %d hosts, want the profile's 3", len(rep.Results))
	}
	// Init is InitEach with no callback, and scans the same hosts.
	if plain, err := New(t.TempDir()).Init(&m, "work", false, false); err != nil || len(plain.Results) != 3 {
		t.Errorf("Init = %d results, %v", len(plain.Results), err)
	}
}
