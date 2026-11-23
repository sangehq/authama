package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/services/query"
)

// The host tree lines its columns up across profiles, and no line ends in
// padding.
func TestListLinesUpItsColumnsAcrossProfiles(t *testing.T) {
	groups := []query.ProfileGroup{
		{Name: "work", Rows: []query.HostRow{
			{Alias: "web1", Hostname: "127.0.0.1", ProviderLabel: "server", KeyName: "work_web1", Status: "not-deployed", Tags: []string{"prod"}},
			{Alias: "github-work", Hostname: "github.com", ProviderLabel: "github/vcs", KeyName: "work_github-work", Status: "deployed"},
		}},
		{Name: "empty", Empty: true},
		{Name: "personal", Rows: []query.HostRow{
			{Alias: "box", Hostname: "10.0.0.2", ProviderLabel: "server", KeyName: "personal", Status: "no-key"},
		}},
	}
	var b bytes.Buffer
	writeHostTree(&b, groups)
	want := "work\n" +
		"  web1         127.0.0.1   [server]      work_web1         (not-deployed)  #prod\n" +
		"  github-work  github.com  [github/vcs]  work_github-work  (deployed)\n" +
		"empty  (no hosts)\n" +
		"personal\n" +
		"  box          10.0.0.2    [server]      personal          (no-key)\n"
	if got := b.String(); got != want {
		t.Fatalf("list printed\n%s\nwant\n%s", got, want)
	}
	for _, line := range strings.Split(b.String(), "\n") {
		if strings.HasSuffix(line, " ") {
			t.Errorf("line ends in padding: %q", line)
		}
	}
}
