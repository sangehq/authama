package envnames

import (
	"sort"
	"strings"
	"testing"
)

func getter(m map[string]string) Getenv { return func(k string) string { return m[k] } }

func lookup(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestSourceAndIsSetReadTheNameItself(t *testing.T) {
	env := map[string]string{ConfigDir: "/new"}
	if v, from := Source(getter(env), ConfigDir); v != "/new" || from != ConfigDir {
		t.Errorf("current: %q from %q", v, from)
	}
	if v, from := Source(getter(map[string]string{}), ConfigDir); v != "" || from != "" {
		t.Errorf("unset: %q from %q", v, from)
	}
	if !IsSet(lookup(map[string]string{GitMode: ""}), GitMode) {
		t.Error("an empty variable is still set")
	}
	if IsSet(lookup(map[string]string{}), GitMode) {
		t.Error("nothing set")
	}
}

// The SSH_MANAGER_* and SSHMGR_* names were removed on 2026-11-07. Each one,
// set alone, must supply nothing: a value under an old name is ignored rather
// than half-honoured. The list is the one the removed table held, so a name
// that comes back under its old spelling fails here.
func TestTheRemovedPreAuthamaNamesAreNotRead(t *testing.T) {
	removed := map[string][]string{
		ConfigDir:        {"SSH_MANAGER_HOME", "SSH_MANAGER_CONFIG_DIR"},
		DevRoot:          {"SSHMGR_DEV_ROOT"},
		AgeRecipient:     {"SSH_MANAGER_AGE_RECIPIENT"},
		AgeIdentityFile:  {"SSH_MANAGER_AGE_IDENTITY_FILE"},
		SnapshotRetain:   {"SSH_MANAGER_SNAPSHOT_RETAIN"},
		OldKeyMaxAgeDays: {"SSH_MANAGER_OLD_KEY_MAX_AGE_DAYS"},
		AutoPin:          {"SSH_MANAGER_AUTO_PIN"},
		History:          {"SSH_MANAGER_HISTORY"},
		GitMode:          {"SSHMGR_GIT_MODE"},
		TokensConfig:     {"SSH_MANAGER_TOKENS_CONFIG"},
		TokensCatalog:    {"SSH_MANAGER_TOKENS_CATALOG"},
		AskpassMode:      {"SSHMGR_ASKPASS_MODE"},
		AskpassSecret:    {"SSHMGR_ASKPASS_SECRET"},
	}
	checked := 0
	for cur, olds := range removed {
		for _, old := range olds {
			if v := Get(getter(map[string]string{old: "x"}), cur); v != "" {
				t.Errorf("$%s still supplies %s: %q", old, cur, v)
			}
			if IsSet(lookup(map[string]string{old: "x"}), cur) {
				t.Errorf("$%s still counts as %s being set", old, cur)
			}
			checked++
		}
	}
	if checked != 14 {
		t.Fatalf("checked %d old names, want 14", checked)
	}
}

func TestEveryNameIsAnAuthamaNameAndTheListIsSorted(t *testing.T) {
	names := Names()
	if !sort.StringsAreSorted(names) {
		t.Error("Names is not sorted")
	}
	for _, n := range names {
		if !strings.HasPrefix(n, "AUTHAMA_") {
			t.Errorf("%s is not an AUTHAMA_ name", n)
		}
	}
}
