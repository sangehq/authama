package events

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// scanned makes a home whose log has a few chained events.
func scanned(t *testing.T) (p0 string, lines []string, verify func() Integrity) {
	t.Helper()
	p, _ := home(t)
	roots := Roots(p, nil)
	t0 := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	for i, name := range []string{"", "a", "b", "c"} {
		if name != "" {
			write(t, filepath.Join(p.SSHDir, name), name+"\n", 0o600)
		}
		if _, err := Scan(p, roots, t0.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(LogFile(p))
	if err != nil {
		t.Fatal(err)
	}
	return LogFile(p), strings.Split(strings.TrimRight(string(b), "\n"), "\n"), func() Integrity {
		in, err := Verify(p)
		if err != nil {
			t.Fatal(err)
		}
		return in
	}
}

func rewrite(t *testing.T, path string, lines []string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// An intact log verifies; an edited, removed or inserted line, or a cut end,
// does not, and says where.
func TestVerifyCatchesEveryKindOfTampering(t *testing.T) {
	path, lines, verify := scanned(t)
	if len(lines) != 4 {
		t.Fatalf("%d lines, want 4", len(lines))
	}
	if in := verify(); !in.OK() || in.Chained != 4 {
		t.Fatalf("intact log: %+v", in)
	}

	edited := append([]string(nil), lines...)
	edited[1] = strings.Replace(edited[1], `"kind":"added"`, `"kind":"removed"`, 1)
	rewrite(t, path, edited)
	if in := verify(); in.OK() || in.Broken != 2 {
		t.Errorf("an edited line: %+v", in)
	}

	rewrite(t, path, append(append([]string(nil), lines[:1]...), lines[2:]...))
	if in := verify(); in.OK() || in.Broken != 2 {
		t.Errorf("a removed line: %+v", in)
	}

	rewrite(t, path, lines[:3])
	if in := verify(); in.OK() || !in.Truncated {
		t.Errorf("a cut end: %+v", in)
	}

	injected := append(append([]string(nil), lines[:2]...), `{"time":"2026-11-04T12:00:00Z","kind":"added","path":"/x"}`)
	rewrite(t, path, append(injected, lines[2:]...))
	if in := verify(); in.OK() || in.Broken != 3 {
		t.Errorf("an injected unchained line: %+v", in)
	}

	rewrite(t, path, lines)
	if in := verify(); !in.OK() {
		t.Errorf("restored log: %+v", in)
	}
}

// A log from before the chain - lines with no hash at its start - is counted
// and then chained onto, not reported as tampered.
func TestVerifyAcceptsALogWrittenBeforeTheChain(t *testing.T) {
	p, _ := home(t)
	if err := os.MkdirAll(p.LogDir(), 0o700); err != nil {
		t.Fatal(err)
	}
	write(t, LogFile(p), `{"time":"2026-11-03T12:00:00Z","kind":"started","detail":"tracking 0 file(s)"}`+"\n", 0o600)
	write(t, filepath.Join(p.SSHDir, "a"), "a\n", 0o600)
	if _, err := Scan(p, Roots(p, nil), time.Now()); err != nil {
		t.Fatal(err)
	}
	in, err := Verify(p)
	if err != nil {
		t.Fatal(err)
	}
	if !in.OK() || in.Unchained != 1 || in.Chained == 0 {
		t.Errorf("%+v", in)
	}
}
