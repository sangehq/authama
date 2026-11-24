package keyaudit

import (
	"slices"
	"testing"
)

// A tool key in the shared profiles/tools/<tool>/ folder belongs to the
// profile that declares it, and is read by its tool, never by a host. The
// audit used to call it untracked (owner guessed from the folder name, "git")
// and unwired (no host names it), and fail doctor on both.
func TestAToolKeyInTheSharedFolderIsNeitherUntrackedNorUnwired(t *testing.T) {
	f := newFixtureFrom(t, `{
	  "version": 1,
	  "defaults": {"key_type": "ed25519", "rotate_after_days": 365},
	  "profiles": {
	    "work": {"key_scope": "per_service",
	      "keys": [{"name": "git-signing", "role": "tool", "tool": "git", "tool_kind": "git-signing"}],
	      "hosts": [{"alias": "gh", "hostname": "github.com", "user": "git", "key_name": "work_gh-ed25519"}]}
	  }
	}`)
	f.writeKey(t, "tools/git", "git-signing", true)
	f.record(t, "SHA256:signing", "~/.ssh/profiles/tools/git/git-signing")
	rep := audit(t, f, false)
	// (The fixture's spare key, which this manifest does not declare, is
	// rightly untracked; only the tool key matters here.)
	if got := subjects(rep, Untracked); slices.Contains(got, "profiles/tools/git/git-signing") {
		t.Errorf("untracked: %v, want no tool key", got)
	}
	if got := subjects(rep, Unwired); slices.Contains(got, "work/git-signing") {
		t.Errorf("unwired: %v, want no tool key", got)
	}
}
