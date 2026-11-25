package change

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/services/mutation"
	"github.com/sangehq/authama/src/internal/services/structure"
)

// The web server auto-applies purpose rules from its watcher, in a process that
// never exits. AutoApplyRuled took the advisory lock and nothing released it,
// so after the first auto-apply every CLI mutation waited on `serve` for good
// (audit F-18, 2026-11-25). AutoApplyForServer runs it as one scoped operation.
func TestAutoApplyForServerReleasesTheLock(t *testing.T) {
	p := home(t)
	if err := os.MkdirAll(p.StateDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	rules := `[{"id":"autorule1","tool":"git","auto_apply":true}]`
	if err := os.WriteFile(filepath.Join(p.StateDir(), "purpose-rules.json"), []byte(rules), 0o600); err != nil {
		t.Fatal(err)
	}
	// A rule-matched, complete purpose suggestion. Its plan names a profile the
	// manifest lacks, so it fails after the guard has taken the lock: the lock
	// has to come back on the failure path too.
	ss := []structure.Suggestion{{
		ID: "s1", Kind: structure.KindPurpose, Confidence: 1, Tool: "ssh",
		Evidence: []string{"rule autorule1"}, Plan: "profile ghost category -",
	}}
	mutation.Release()
	_, _ = AutoApplyForServer(p, ss, io.Discard, history.SourceWeb)
	if mutation.Held() {
		t.Error("the advisory lock is still held after a server auto-apply; CLI mutations would block on serve")
	}
}

// The watcher calls AutoApplyForServer after every change it sees, and almost
// always nothing matches. With nothing to apply it must not queue behind a
// running web job or open a history scope: it did both, so a job waited on
// every file change and a bare-run test saw a scope it never opened
// (found by a shuffled run, 2026-11-25).
func TestAutoApplyForServerWithNothingToApplyTakesNoLock(t *testing.T) {
	p := home(t)
	// Hold the jobs' lock as a running web job would (waiting for any other
	// test that holds it first).
	release, err := mutation.Exclusive(busyWait)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	start := time.Now()
	ids, err := AutoApplyForServer(p, nil, io.Discard, history.SourceWeb)
	if err != nil || len(ids) != 0 {
		t.Fatalf("nothing to apply: ids %v, err %v", ids, err)
	}
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("it waited %v behind a running job with nothing to apply", took)
	}
	if d := mutation.Depth(); d != 0 {
		t.Errorf("%d history scope(s) left open", d)
	}
}

// What to move is decided before waiting for a running web job, so a rule the
// owner turned off while it waited must not still be applied: it is decided
// again under the lock (review, 2026-11-25).
func TestAutoApplyForServerHonoursARuleTurnedOffWhileItWaited(t *testing.T) {
	p := home(t)
	if err := os.MkdirAll(p.StateDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	rulesFile := filepath.Join(p.StateDir(), "purpose-rules.json")
	if err := os.WriteFile(rulesFile, []byte(`[{"id":"autorule1","tool":"git","auto_apply":true}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	ss := []structure.Suggestion{{
		ID: "s1", Kind: structure.KindPurpose, Confidence: 1, Tool: "ssh",
		Evidence: []string{"rule autorule1"}, Plan: "profile ghost category -",
	}}
	release, err := mutation.Exclusive(busyWait)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		ids []string
		err error
	}
	done := make(chan result, 1)
	go func() {
		ids, err := AutoApplyForServer(p, ss, io.Discard, history.SourceWeb)
		done <- result{ids, err}
	}()
	time.Sleep(200 * time.Millisecond) // it has picked its moves and is waiting
	if err := os.WriteFile(rulesFile, []byte(`[{"id":"autorule1","tool":"git","auto_apply":false}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	release()
	r := <-done
	if r.err != nil || len(r.ids) != 0 {
		t.Errorf("a rule turned off while it waited was still applied: ids %v, err %v", r.ids, r.err)
	}
	if snaps, _ := filepath.Glob(filepath.Join(p.SnapshotsDir(), "*")); len(snaps) != 0 {
		t.Errorf("a snapshot was taken for a move that should not run: %v", snaps)
	}
}
