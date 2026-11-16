package mutation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/util/log"
	"github.com/sangehq/authama/src/internal/util/paths"
)

// splitHome is the default layout: config and state in separate homes.
func splitHome(t *testing.T) paths.Paths {
	t.Helper()
	root := t.TempDir()
	p := paths.Paths{SSHDir: filepath.Join(root, "ssh"), ConfigDir: filepath.Join(root, "cfg"), StateRoot: filepath.Join(root, "state")}
	if err := os.MkdirAll(p.ConfigDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.Manifest(), []byte(`{"profiles":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	prev := log.SetSink(sinkEvent)
	t.Cleanup(func() { log.SetSink(prev) })
	return p
}

func recorded(t *testing.T, p paths.Paths) []history.Op {
	t.Helper()
	s, err := history.Open(p.HistoryDB())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	var out []history.Op
	ops, err := s.Query(history.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range ops {
		if o.Source != history.SourceBackfill {
			out = append(out, o)
		}
	}
	return out
}

// A locked operation's events land in it when state is not under config
// (audit B-01).
func TestALockedOperationKeepsItsEventsInASplitLayout(t *testing.T) {
	p := splitHome(t)
	Begin(Scope{Command: "host add", Source: history.SourceCLI})
	opening(p)
	log.Audit(p.AuditLog(), "host.add", log.Field{Key: "host", Value: "gh"})
	End(Outcome{})

	ops := recorded(t, p)
	if len(ops) != 1 || len(ops[0].Events) != 1 || ops[0].Events[0].Event != "host.add" {
		t.Fatalf("ops = %+v, want one operation holding the host.add event", ops)
	}
}

// An event raised before the scope took the lock opens the operation against
// the scope's own homes, so its manifest hash reads the real manifest.
func TestAnUnlockedEventOpensTheOperationAgainstTheScopesHomes(t *testing.T) {
	p := splitHome(t)
	Begin(Scope{Command: "load", Source: history.SourceCLI, Paths: p})
	log.Audit(p.AuditLog(), "agent.load", log.Field{Key: "profile", Value: "work"})
	End(Outcome{})

	ops := recorded(t, p)
	if len(ops) != 1 || len(ops[0].Events) != 1 {
		t.Fatalf("ops = %+v, want one operation holding the event", ops)
	}
	if ops[0].ManifestBefore == "" || ops[0].ManifestAfter == "" {
		t.Errorf("manifest before %q after %q: the operation was opened without the config home",
			ops[0].ManifestBefore, ops[0].ManifestAfter)
	}
}
