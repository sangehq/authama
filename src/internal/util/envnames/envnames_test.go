package envnames

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

func getter(m map[string]string) Getenv { return func(k string) string { return m[k] } }

func lookup(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func TestTheCurrentNameWinsAndLegacyNamesStillCount(t *testing.T) {
	env := map[string]string{"SSH_MANAGER_HOME": "/old", "SSH_MANAGER_CONFIG_DIR": "/older"}
	if v, from := Source(getter(env), ConfigDir); v != "/old" || from != "SSH_MANAGER_HOME" {
		t.Errorf("legacy: %q from %q", v, from)
	}
	env[ConfigDir] = "/new"
	if v, from := Source(getter(env), ConfigDir); v != "/new" || from != ConfigDir {
		t.Errorf("current: %q from %q", v, from)
	}
	if !IsSet(lookup(map[string]string{"SSHMGR_GIT_MODE": ""}), GitMode) {
		t.Error("an empty legacy variable is still set")
	}
	if IsSet(lookup(map[string]string{}), GitMode) {
		t.Error("nothing set")
	}
}

func TestDeprecatedNamesTheRenamesButNotInternalOnes(t *testing.T) {
	got := Deprecated(getter(map[string]string{
		"SSH_MANAGER_AGE_RECIPIENT": "age1", "SSHMGR_ASKPASS_MODE": "1", "AUTHAMA_HISTORY": "0",
	}))
	if want := []string{"SSH_MANAGER_AGE_RECIPIENT -> AUTHAMA_AGE_RECIPIENT"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Deprecated = %v, want %v", got, want)
	}
}

func TestEveryNameIsAnAuthamaNameAndListsAreSorted(t *testing.T) {
	names := Names()
	if !sort.StringsAreSorted(names) || !sort.StringsAreSorted(AllNames()) {
		t.Error("lists are not sorted")
	}
	for _, n := range names {
		if !strings.HasPrefix(n, "AUTHAMA_") {
			t.Errorf("%s is not an AUTHAMA_ name", n)
		}
		for _, l := range Legacy(n) {
			if strings.HasPrefix(l, "AUTHAMA_") {
				t.Errorf("legacy name %s of %s is current", l, n)
			}
		}
	}
}
