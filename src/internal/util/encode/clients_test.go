package encode

import (
	"strings"
	"testing"
)

// Each client's escaping, for its own hazard: a value must stay a value.
// These moved here with the writers from core/renderer.
func TestClientValuesAreEscapedForEachClient(t *testing.T) {
	if xmlText(`a<b&"c`) != "a&lt;b&amp;&#34;c" {
		t.Errorf("SecureCRT XML escaping: %q", xmlText(`a<b&"c`))
	}
	if got := mobaField("a%b#c=d\ne"); got != "a_b_c-d e" {
		t.Errorf("MobaXterm: a separator in a value shifts the fields: %q", got)
	}
	if puttyName(`a b\c*%`) != `a%20b%5Cc%2A%25` {
		t.Errorf("PuTTY session name escaping: %s", puttyName(`a b\c*%`))
	}
	if remminaValue(" a\\b\n") != `\sa\\b\n` {
		t.Errorf("Remmina escaping: %q", remminaValue(" a\\b\n"))
	}
	if cell("=1+1") != "'=1+1" || termiusTags([]string{"a,b", "-c"}) != "a b,'-c" {
		t.Errorf("formula cells: %q %q", cell("=1+1"), termiusTags([]string{"a,b", "-c"}))
	}
}

// Termius reads '/' in Groups as nesting: each scheme writes its path, a part
// a host lacks is left out rather than written as an empty level, and a '/'
// inside one part cannot add a level.
func TestTermiusGroupSchemes(t *testing.T) {
	full := Host{Group: "simtabi", Category: "orgs", Env: "prd"}
	bare := Host{Group: "legacy"}
	for scheme, want := range map[string][2]string{
		"":                              {"simtabi", "legacy"},
		TermiusGroupsProfile:            {"simtabi", "legacy"},
		TermiusGroupsCategoryProfile:    {"orgs/simtabi", "legacy"},
		TermiusGroupsProfileEnv:         {"simtabi/prd", "legacy"},
		TermiusGroupsCategoryProfileEnv: {"orgs/simtabi/prd", "legacy"},
		TermiusGroupsNone:               {"", ""},
	} {
		if got := [2]string{TermiusGroup(full, scheme), TermiusGroup(bare, scheme)}; got != want {
			t.Errorf("%q: got %q, want %q", scheme, got, want)
		}
	}
	if got := TermiusGroup(Host{Group: "a/b", Env: "x/y"}, TermiusGroupsProfileEnv); got != "a-b/x-y" {
		t.Errorf("a '/' inside a part nests: %q", got)
	}
	for _, s := range TermiusGroupSchemes {
		if err := CheckTermiusGroups(s); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
	if err := CheckTermiusGroups("env"); err == nil || !strings.Contains(err.Error(), "profile/env") {
		t.Errorf("an unknown scheme should be refused naming the choices: %v", err)
	}
}

// The CSV follows Termius's own template: its header, ssh in every row, the
// group as a path, several tags in one quoted cell, an empty port left empty
// (Termius then uses 22), and never a password.
func TestTermiusCSVFollowsTheTemplate(t *testing.T) {
	got, err := TermiusCSV([]Host{
		{Group: "Project", Env: "Dev", Alias: "Web Server", Hostname: "192.168.1.1", Port: 2300, User: "admin", Tags: []string{"dev"}},
		{Group: "Project", Env: "Prod", Alias: "web-prod", Hostname: "serverauditor.com", User: "root", Tags: []string{"prod"}},
		{Alias: "Database", Hostname: "db.serverauditor.com", Port: 4567, User: "dbuser", Tags: []string{"db", "mysql"}},
	}, TermiusGroupsProfileEnv)
	if err != nil {
		t.Fatal(err)
	}
	want := "Groups,Label,Tags,Hostname/IP,Protocol,Port,Username,Password\n" +
		"Project/Dev,Web Server,dev,192.168.1.1,ssh,2300,admin,\n" +
		"Project/Prod,web-prod,prod,serverauditor.com,ssh,,root,\n" +
		",Database,\"db,mysql\",db.serverauditor.com,ssh,4567,dbuser,\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
	if _, err := TermiusCSV(nil, "nope"); err == nil {
		t.Error("an unknown scheme was accepted")
	}
}
