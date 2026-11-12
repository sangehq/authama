package rotator

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/simtabi/authama/src/internal/util/paths"
)

// designRun is the run the summary was designed from: ten keys over four
// owners, three that cannot rotate, three that take the key by hand.
func designRun() []CheckReport {
	never := "answers, but the current key does not log in: it was never deployed here, so there is nothing to rotate yet; deploy it first (the key's page, Deploy)"
	paste := "ploi takes the new key by hand: the rotation will stop and ask you to paste it"
	ok := func(key, alias, via, kind string) CheckReport {
		return CheckReport{Key: key, Hosts: []HostCheck{{Alias: alias, Via: via, OK: true, Kind: kind}}}
	}
	return []CheckReport{
		{Key: "simtabi/simtabi_oribi-web-ed25519", Hosts: []HostCheck{{Alias: "oribi-web", Via: "ploi", OK: true, Manual: true, Kind: KindManual, Message: paste}}},
		{Key: "simtabi/simtabi_oribi-db-maria-ed25519", Hosts: []HostCheck{{Alias: "oribi-db-maria", Via: "ploi", OK: true, Manual: true, Kind: KindManual, Message: paste}}},
		{Key: "simtabi/simtabi_oribi-db-psql-ed25519", Hosts: []HostCheck{{Alias: "oribi-db-psql", Via: "ploi", OK: true, Manual: true, Kind: KindManual, Message: paste}}},
		{Key: "simtabi/simtabi_simbi-prd-web-00-ed25519", Hosts: []HostCheck{{Alias: "simbi-prd-web-00", Via: "ssh", Kind: KindNeverDeployed, Message: never}}},
		{Key: "simtabi/simtabi_simbi-prd-db-00-ed25519", Hosts: []HostCheck{{Alias: "simbi-prd-db-00", Via: "ssh", Kind: KindNeverDeployed, Message: never}}},
		ok("imanimanyara/imanimanyara_github-ed25519", "github-imanimanyara", "github api", KindAPI),
		{Key: "imanimanyara/docker-tunnel", Hosts: []HostCheck{{Alias: "incus", Via: "ssh", Kind: KindUnreachable, Message: "incus:22 unreachable - check your network connection"}}},
		ok("dannielagumo/dannielagumo_github-ed25519", "github-dannielagumo", "github api", KindAPI),
		ok("adelsaiq/adelsaiq_dani-prd-web-00-ed25519", "dani-prd-web-00", "ssh", KindLogsIn),
		ok("adelsaiq/adelsaiq_dani-prd-db-00-ed25519", "dani-prd-db-00", "ssh", KindLogsIn),
	}
}

func TestADryRunSummaryGroupsSortsAndNumbersItsNotes(t *testing.T) {
	s := SummarizeCheck(designRun(), 12400*time.Millisecond)
	s.Command = "authama ssh rotate --all --check"
	out := s.Format()
	if os.Getenv("SHOW_SUMMARY") != "" {
		t.Log("\n" + out)
	}
	for _, want := range []string{
		"$ authama ssh rotate --all --check",
		"Result   3 would fail · 3 would pause · 4 would rotate   of 10 keys · 4 owners",
		"\nrotate\n├─ simtabi", "│  ├─ simbi · prd", "│  │  ├─ × web-00", "│  └─ oribi", "│     ├─ ! web",
		"\n│\n├─ imanimanyara", "└─ adelsaiq",
		"awaiting paste [1]", "never deployed [2]", "host unreachable [3]",
		"Needs you", "× 1  check network  [3]   imanimanyara: docker-tunnel", "× 2  deploy first   [2]   simtabi/simbi·prd: web-00, db-00",
		"! 3  paste by hand  [1]   simtabi/oribi: web, db-maria, db-psql",
		"3 would fail · 3 would pause · 4 would rotate  ·  12.4s",
		"re-run failed: authama ssh rotate --failed --check",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the summary lacks %q:\n%s", want, out)
		}
	}
	// Worst first: the owner with failures before the ones that went through,
	// and within simtabi the failing project before the pausing one.
	if strings.Index(out, "─ simtabi") > strings.Index(out, "─ adelsaiq") || strings.Index(out, "simbi · prd") > strings.Index(out, "oribi") {
		t.Errorf("the tree is not worst first:\n%s", out)
	}
	// A single key with no project shares no group: docker-tunnel stands alone.
	if strings.Contains(out, "docker ·") || strings.Contains(out, "─ docker\n") {
		t.Errorf("docker-tunnel was grouped:\n%s", out)
	}
	// Each note is printed once.
	if strings.Count(out, "takes the new key by hand") != 1 {
		t.Errorf("a shared note is printed more than once:\n%s", out)
	}
}

func TestARunSummarySaysWhatWasLeftAndWhatWasReplaced(t *testing.T) {
	results := []RunResult{
		{Ref: "work/work_gh-ed25519", Via: "github api", Report: &RotateReport{Committed: true}},
		{Ref: "work/work_box-ed25519", Via: "ssh", Err: errors.New("box:22 unreachable - check your network connection")},
		{Ref: "work/work_vm-ed25519", Via: "ssh", Err: errors.New("deploy failed")},
	}
	s := SummarizeRun(results, []string{"work/work_vm-ed25519"}, 0)
	out := s.Format()
	for _, want := range []string{"Result   1 not rotated · 1 replaced · 1 rotated", "host unreachable [1]", "deploy next [2]",
		"Keys marked not rotated were left untouched", "re-run failed: authama ssh rotate --failed"} {
		if !strings.Contains(out, want) {
			t.Errorf("the run summary lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "--check") {
		t.Error("a rotation's re-run command is a dry run")
	}
}

// rotate --failed runs again what the last run left: the list is kept, read
// back, and cleared by a run that leaves nothing.
func TestTheFailedKeysAreKeptForTheNextRun(t *testing.T) {
	p := paths.Paths{ConfigDir: t.TempDir()}
	if got, err := LoadFailed(p); err != nil || got != nil {
		t.Fatalf("before any run: %v, %v", got, err)
	}
	if err := SaveFailed(p, []string{"work/a", "work/b"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := LoadFailed(p); strings.Join(got, ",") != "work/a,work/b" {
		t.Errorf("read back %v", got)
	}
	if err := SaveFailed(p, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := LoadFailed(p); got != nil {
		t.Errorf("a clean run left %v", got)
	}
}

// While a run is under way the summary keeps the run's order, marks the key
// under way and the ones waiting, and offers no re-run and no closing note
// until it ends.
func TestASummaryDrawnWhileRunningKeepsTheRunsOrder(t *testing.T) {
	reports := designRun()[:4] // three that pause, then one that fails
	s := SummarizeCheck(reports, time.Second)
	s.Keys = append(s.Keys, Pending([]string{"imanimanyara/docker-tunnel", "adelsaiq/adelsaiq_dani-prd-web-00-ed25519"}, "testing")...)
	if !s.Running() {
		t.Fatal("a summary with pending keys is not running")
	}
	out := s.Format()
	for _, want := range []string{"› docker-tunnel", "testing", "· web-00", "waiting"} {
		if !strings.Contains(out, want) {
			t.Errorf("the running summary lacks %q:\n%s", want, out)
		}
	}
	// Run order: oribi (paused) before simbi (failed), as they ran.
	if strings.Index(out, "oribi") > strings.Index(out, "simbi") {
		t.Errorf("the running summary was resorted:\n%s", out)
	}
	if s.Rerun() != "" || s.Untouched() != "" {
		t.Errorf("a running summary offers a re-run (%q) or a closing note (%q)", s.Rerun(), s.Untouched())
	}
}
