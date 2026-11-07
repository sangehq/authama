package cli

import (
	"strings"
	"testing"
)

// The list commands print CSV, header first; a token list never puts a
// secret in one.
func TestListCommandsPrintCSV(t *testing.T) {
	editHome(t)
	run(t, "ssh", "profile", "add", "work")
	run(t, "ssh", "host", "add", "work", "web1", "-H", "web1.example", "-u", "deploy")
	out := run(t, "ssh", "list", "--csv")
	if !strings.HasPrefix(out, "profile,alias,hostname,provider,key,status,tags\n") || !strings.Contains(out, "work,web1,web1.example,") {
		t.Errorf("ssh list --csv:\n%s", out)
	}
	if out := run(t, "ssh", "key", "list", "--csv"); !strings.HasPrefix(out, "key,profile,type,") {
		t.Errorf("ssh key list --csv:\n%s", out)
	}
	if out := run(t, "events", "--csv"); !strings.HasPrefix(out, "time,seen,kind,area,path,") {
		t.Errorf("events --csv:\n%s", out)
	}
	if out := run(t, "history", "--csv"); !strings.HasPrefix(out, "op_id,started_at,") || !strings.Contains(out, "host add") {
		t.Errorf("history --csv:\n%s", out)
	}
	if _, err := runIn(t, "", "token", "list", "--csv", "--reveal", "--yes"); err == nil || !strings.Contains(err.Error(), "never holds a secret") {
		t.Errorf("token list --csv --reveal: %v", err)
	}
}
