package scheduler

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildPlist(t *testing.T) {
	p := buildPlist(Label, `"/opt/my apps/authama" audit --notify`)
	for _, want := range []string{
		"<key>Label</key><string>authama.expiry</string>",
		"        <string>/opt/my apps/authama</string>", // quoted path stays one arg
		"        <string>audit</string>",
		"        <string>--notify</string>",
		"<key>Hour</key><integer>9</integer><key>Minute</key><integer>0</integer>",
		"<key>RunAtLoad</key><false/>",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("plist missing %q\n%s", want, p)
		}
	}
}

func TestBuildService(t *testing.T) {
	s := buildService("/x/authama audit --notify")
	if !strings.Contains(s, "ExecStart=/x/authama audit --notify") {
		t.Errorf("service unit wrong:\n%s", s)
	}
	if !strings.Contains(buildService("a %p b"), "ExecStart=a %%p b") {
		t.Error("a literal %% should be escaped in ExecStart")
	}
}

func TestShlexSplit(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{`/usr/bin/authama audit --notify`, []string{"/usr/bin/authama", "audit", "--notify"}},
		{`"/opt/my apps/authama" audit`, []string{"/opt/my apps/authama", "audit"}},
	}
	for _, c := range cases {
		if got := shlexSplit(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("shlexSplit(%q)=%v want %v", c.in, got, c.want)
		}
	}
}

// The job moved to a new label at 0.1.2; installing removes the old one, which
// only works while the two differ.
func TestLabels(t *testing.T) {
	if Label != "authama.expiry" || LegacyLabel != "ssh_manager.expiry" {
		t.Errorf("Label=%q LegacyLabel=%q", Label, LegacyLabel)
	}
	if got := schtasksDeleteArgs(LegacyLabel); !reflect.DeepEqual(got, []string{"schtasks", "/Delete", "/TN", "ssh_manager.expiry", "/F"}) {
		t.Errorf("delete argv = %q", got)
	}
}

func TestVMSyncJobIsItsOwn(t *testing.T) {
	if s := buildServiceFor(VMSyncLabel, "/x/authama ssh vm sync --quiet --yes"); !strings.Contains(s, "Description=authama VM host sync") {
		t.Fatalf("service: %s", s)
	}
	if s := timerUnitFor(VMSyncLabel); !strings.Contains(s, "OnCalendar=*-*-* 09:30:00") {
		t.Fatalf("timer: %s", s)
	}
	if p := buildPlist(VMSyncLabel, "/x/authama ssh vm sync"); !strings.Contains(p, "<integer>9</integer><key>Minute</key><integer>30</integer>") {
		t.Fatalf("plist: %s", p)
	}
	if p := buildPlist(Label, "/x/authama audit --notify"); !strings.Contains(p, "<integer>9</integer><key>Minute</key><integer>0</integer>") {
		t.Fatalf("notifier plist moved: %s", p)
	}
	if a := schtasksArgs("x", VMSyncLabel); a[9] != "09:30" {
		t.Fatalf("schtasks: %v", a)
	}
}

func TestTheExportsJobIsItsOwn(t *testing.T) {
	if s := buildServiceFor(ExportsLabel, ExportsCommand()); !strings.Contains(s, "Description=authama scheduled exports") ||
		!strings.Contains(s, "export schedule run --quiet") {
		t.Fatalf("service: %s", s)
	}
	if s := timerUnitFor(ExportsLabel); !strings.Contains(s, "OnCalendar=*-*-* 10:00:00") {
		t.Fatalf("timer: %s", s)
	}
	if p := buildPlist(ExportsLabel, "/x/authama export schedule run"); !strings.Contains(p, "<integer>10</integer><key>Minute</key><integer>0</integer>") {
		t.Fatalf("plist: %s", p)
	}
	if a := schtasksArgs("x", ExportsLabel); a[9] != "10:00" {
		t.Fatalf("schtasks: %v", a)
	}
}
