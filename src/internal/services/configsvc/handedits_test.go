package configsvc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const orgsManifest = `{"version":1,"profiles":{"simtabi":{"category":"orgs","hosts":[
	{"alias":"oribi-web","hostname":"143.198.186.131","user":"ploi"}]}}}`

// What Sim pasted into the managed orgs file by hand.
const pastedSimbi = `
# simbi prod: private IPs over Cloudflare WARP (WARP must be connected)
Host warp-simbi-prd-web-00
    HostName 10.116.0.3
    User ploi
    IdentityFile ~/.ssh/profiles/orgs/simtabi/simtabi_simbi-prd-web-00-ed25519
    IdentitiesOnly yes
    ProxyCommand none
    ConnectTimeout 10
`

func orgsHome(t *testing.T) (ssh, state, orgs string, svc func() *Service) {
	t.Helper()
	ssh, state = t.TempDir(), t.TempDir()
	m := load(t, orgsManifest)
	svc = func() *Service { return New(ssh, m, false).WithState(state) }
	if _, err := svc().Write(false); err != nil {
		t.Fatal(err)
	}
	return ssh, state, filepath.Join(ssh, "profiles", "orgs", "config"), svc
}

func appendTo(t *testing.T, path, text string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(b, text...), 0o600); err != nil {
		t.Fatal(err)
	}
}

// A render no longer overwrites what was typed into a managed file: it
// refuses, names each edit and says how to keep or discard it.
func TestARenderRefusesToDiscardHandEdits(t *testing.T) {
	_, _, orgs, svc := orgsHome(t)
	appendTo(t, orgs, pastedSimbi)
	before, _ := os.ReadFile(orgs)

	_, err := svc().Write(false)
	var he *HandEditsError
	if !errors.As(err, &he) {
		t.Fatalf("want a HandEditsError, got %v", err)
	}
	msg := err.Error()
	for _, want := range []string{"profiles/orgs/config: warp-simbi-prd-web-00: adds the heading # simbi prod: private IPs",
		"warp-simbi-prd-web-00: a Host block the manifest does not have", "adopt-edits", "render --force"} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in:\n%s", want, msg)
		}
	}
	if after, _ := os.ReadFile(orgs); string(after) != string(before) {
		t.Error("the refused render changed the file")
	}
	res, err := svc().Write(true)
	if err != nil || len(res.HandEdits) != 2 {
		t.Errorf("a dry run reports the edits without refusing: %v %v", res, err)
	}
	chk, err := svc().Check(false)
	if err != nil || len(chk.HandEdits) != 2 || !strings.Contains(chk.Format(), "HAND-EDIT profiles/orgs/config: warp-simbi") {
		t.Errorf("check reports them: %v\n%s", err, chk.Format())
	}
}

// --force replaces them, after copying the edited file aside.
func TestAForcedRenderBacksTheEditsUp(t *testing.T) {
	_, _, orgs, svc := orgsHome(t)
	appendTo(t, orgs, pastedSimbi)
	edited, _ := os.ReadFile(orgs)
	backup := filepath.Join(t.TempDir(), "bk")
	res, err := svc().Force(backup).Write(false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Backup != backup || len(res.HandEdits) == 0 {
		t.Errorf("result: %+v", res)
	}
	if b, err := os.ReadFile(filepath.Join(backup, "profiles", "orgs", "config")); err != nil || string(b) != string(edited) {
		t.Errorf("backup: %v %q", err, b)
	}
	if b, _ := os.ReadFile(orgs); strings.Contains(string(b), "warp-simbi") {
		t.Error("the forced render kept the edit")
	}
	if _, err := svc().Write(false); err != nil {
		t.Errorf("after the forced render the next one is clean: %v", err)
	}
}

// With the record, a value changed by hand is an edit too; a render of a
// changed manifest is not.
func TestTheRecordTellsAuthamasLinesFromHandEdits(t *testing.T) {
	ssh, state, orgs, _ := orgsHome(t)
	b, _ := os.ReadFile(orgs)
	if err := os.WriteFile(orgs, []byte(strings.Replace(string(b), "143.198.186.131", "10.0.0.9", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	m := load(t, orgsManifest)
	edits, err := New(ssh, m, false).WithState(state).HandEdits()
	if err != nil || len(edits) != 1 || !strings.Contains(edits[0].What, "sets HostName 10.0.0.9 (authama wrote 143.198.186.131)") {
		t.Errorf("edits: %v %v", edits, err)
	}
	// The manifest moving on (host renamed, address changed) is authama's own.
	if err := os.WriteFile(orgs, b, 0o600); err != nil {
		t.Fatal(err)
	}
	m2 := load(t, strings.Replace(orgsManifest, "143.198.186.131", "10.9.9.9", 1))
	if _, err := New(ssh, m2, false).WithState(state).Write(false); err != nil {
		t.Errorf("a manifest change is not a hand edit: %v", err)
	}
}

// Before the record existed, a block in the shape authama writes is an older
// render (a host deleted since), and only what the manifest has no trace of
// is an edit.
func TestWithoutARecordOnlyForeignLinesAreEdits(t *testing.T) {
	ssh, state, orgs, _ := orgsHome(t)
	if err := os.Remove(filepath.Join(state, RecordFile)); err != nil {
		t.Fatal(err)
	}
	empty := load(t, `{"version":1,"profiles":{"simtabi":{"category":"orgs","hosts":[
		{"alias":"oribi-db","hostname":"161.35.104.205","user":"ploi"}]}}}`)
	if _, err := New(ssh, empty, false).WithState(state).Write(false); err != nil {
		t.Errorf("the deleted host's rendered block is not a hand edit: %v", err)
	}
	if err := os.Remove(filepath.Join(state, RecordFile)); err != nil {
		t.Fatal(err)
	}
	appendTo(t, orgs, pastedSimbi)
	if _, err := New(ssh, empty, false).WithState(state).Write(false); !IsHandEdits(err) {
		t.Errorf("a pasted block is: %v", err)
	}
}

// Without WithState nothing is guarded or recorded, as before.
func TestAServiceWithoutStateWritesAsBefore(t *testing.T) {
	ssh, _, orgs, _ := orgsHome(t)
	appendTo(t, orgs, pastedSimbi)
	if _, err := New(ssh, load(t, orgsManifest), false).Write(false); err != nil {
		t.Fatal(err)
	}
}
