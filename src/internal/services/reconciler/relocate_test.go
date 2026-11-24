package reconciler

import (
	"encoding/json"
	"testing"

	"github.com/sangehq/authama/src/internal/core/inventory"
	"github.com/sangehq/authama/src/internal/core/manifest"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// A key whose files moved (a tool key into profiles/tools/) keeps its record:
// reconcile used to see its fingerprint already recorded under the old path,
// call that "not ours to move", and leave the key unrecorded and the old
// record stale for good. A record whose path another manifest key still owns
// stays where it is.
func TestRecordExistingFollowsAMovedKey(t *testing.T) {
	var m manifest.Manifest
	if err := json.Unmarshal([]byte(`{"version":1,"defaults":{"key_type":"ed25519","rotate_after_days":365},"profiles":{
	  "me":{"category":"personal","key_scope":"per_service",
	    "keys":[{"name":"git-signing","role":"tool","tool":"git","tool_kind":"git-signing"}],
	    "hosts":[{"alias":"gh","hostname":"github.com","user":"git"}]}}}`), &m); err != nil {
		t.Fatal(err)
	}
	p := paths.Paths{SSHDir: t.TempDir(), ConfigDir: t.TempDir()}
	inv := inventory.New()
	ref := manifest.KeyRef{Profile: "me", KeyName: "git-signing"}
	if mk, err := New(p, &m, inv, false).MintRef(ref, ""); err != nil || mk == nil {
		t.Fatalf("mint: %v %v", mk, err)
	}
	newPath := m.IdentityFile("me", "git-signing")
	fps := recordsAt(inv, newPath)
	if len(fps) != 1 {
		t.Fatalf("records at %s: %v", newPath, fps)
	}
	fp := fps[0]
	deployed := []inventory.Deployment{{Target: "github", Method: "web-panel", Verified: true}}

	// The record still names where the key used to be, which no key owns.
	rec := inv.Keys[fp]
	rec.Path, rec.Deployments = "~/.ssh/profiles/personal/me/tools/git/git-signing", deployed
	inv.Keys[fp] = rec
	if ok, err := New(p, &m, inv, false).RecordExisting(ref); err != nil || !ok {
		t.Fatalf("RecordExisting: %v %v, want the record moved", ok, err)
	}
	if got := inv.Keys[fp]; got.Path != newPath || len(got.Deployments) != 1 {
		t.Fatalf("record = %+v, want it at %s with its deployment", got, newPath)
	}

	// The record names a path another manifest key owns: it is that key's.
	owned := ""
	refs, _ := m.KeyRefs()
	for _, r := range refs {
		if r != ref {
			owned = m.IdentityFile(r.Profile, r.KeyName)
		}
	}
	if owned == "" {
		t.Fatal("the manifest has no second key to own a path")
	}
	rec = inv.Keys[fp]
	rec.Path = owned
	inv.Keys[fp] = rec
	if ok, _ := New(p, &m, inv, false).RecordExisting(ref); ok || inv.Keys[fp].Path != owned {
		t.Fatalf("moved a record another key owns: %+v", inv.Keys[fp])
	}
}
