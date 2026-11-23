package perms

import (
	"strings"
	"testing"
)

const testUser = "S-1-5-21-1-2-3-1001"

// What icacls leaves (you, plus SYSTEM and Administrators) passes; any other
// principal with access fails, and says who.
func TestACLOwnerOnlyRules(t *testing.T) {
	mine := []aclEntry{{Allow: true, SID: testUser}, {Allow: true, SID: sidSystem}, {Allow: true, SID: sidAdministrators}}
	cases := []struct {
		name    string
		owner   string
		null    bool
		entries []aclEntry
		ok      bool
		whyHas  string
	}{
		{"only you", testUser, false, mine[:1], true, ""},
		{"you, SYSTEM and Administrators", testUser, false, mine, true, ""},
		{"owned by Administrators", sidAdministrators, false, mine, true, ""},
		{"an empty DACL: no one", testUser, false, nil, true, ""},
		{"Everyone can read", testUser, false, append(mine, aclEntry{Allow: true, SID: "S-1-1-0"}), false, "Everyone"},
		{"Users can read", testUser, false, append(mine, aclEntry{Allow: true, SID: "S-1-5-32-545"}), false, "Users"},
		{"Authenticated Users", testUser, false, append(mine, aclEntry{Allow: true, SID: "S-1-5-11"}), false, "Authenticated Users"},
		{"another account", testUser, false, append(mine, aclEntry{Allow: true, SID: "S-1-5-21-1-2-3-1002"}), false, "1002"},
		{"a deny for Everyone is fine", testUser, false, append(mine, aclEntry{Allow: false, SID: "S-1-1-0"}), true, ""},
		{"inherit-only for children is fine", testUser, false, append(mine, aclEntry{Allow: true, InheritOnly: true, SID: "S-1-3-0"}), true, ""},
		{"no DACL at all", testUser, true, nil, false, "no DACL"},
		{"owned by someone else", "S-1-5-21-1-2-3-1002", false, mine, false, "owned by"},
	}
	for _, c := range cases {
		ok, why := aclOwnerOnly(testUser, c.owner, c.null, c.entries)
		if ok != c.ok || (c.whyHas != "" && !strings.Contains(why, c.whyHas)) {
			t.Errorf("%s: ok=%v why=%q, want ok=%v naming %q", c.name, ok, why, c.ok, c.whyHas)
		}
	}
	if ok, _ := aclOwnerOnly("", testUser, false, nil); ok {
		t.Error("an unknown current user passed")
	}
}
