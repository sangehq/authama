package fs

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSegmentKeepsOnlyOnePlainElement(t *testing.T) {
	for in, want := range map[string]string{
		"work": "work", "id_ed25519.pub": "id_ed25519.pub", "a-b_c.d": "a-b_c.d",
		"": InvalidSegment, ".": InvalidSegment, "..": InvalidSegment, "../x": InvalidSegment,
		"a/b": InvalidSegment, `a\b`: InvalidSegment, "/etc": InvalidSegment, "x\x00y": InvalidSegment,
	} {
		if got := Segment(in); got != want {
			t.Errorf("Segment(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLocalRelStaysInsideItsRoot(t *testing.T) {
	for in, want := range map[string]string{
		"profiles": "profiles", "profiles/work/id": "profiles/work/id", "a/./b": "a/b", "a/b/../c": "a/c",
		"": InvalidSegment, "..": InvalidSegment, "../x": InvalidSegment, "a/../../x": InvalidSegment,
		"/abs": InvalidSegment, "a\x00b": InvalidSegment,
	} {
		if got := LocalRel(in); got != want {
			t.Errorf("LocalRel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWithinRefusesAPathOutsideItsRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "ssh")
	for _, ok := range []string{root, filepath.Join(root, "a"), filepath.Join(root, "a", "..", "b")} {
		if _, err := Within(root, ok); err != nil {
			t.Errorf("Within(%q): %v", ok, err)
		}
	}
	for _, bad := range []string{filepath.Dir(root), filepath.Join(root, "..", "x"), root + "x", filepath.Join(root+"x", "y")} {
		if _, err := Within(root, bad); !errors.Is(err, ErrOutside) {
			t.Errorf("Within(%q) = %v, want ErrOutside", bad, err)
		}
	}
}

func TestAbsDirRefusesControlCharacters(t *testing.T) {
	if got, err := AbsDir("rel/dir"); err != nil || !filepath.IsAbs(got) {
		t.Errorf("AbsDir(rel/dir) = %q, %v", got, err)
	}
	for _, bad := range []string{"/srv/a\nb", "/srv/\x00", "/srv/a\x7f"} {
		if _, err := AbsDir(bad); err == nil {
			t.Errorf("AbsDir(%q) accepted", bad)
		}
	}
	if runtime.GOOS == "windows" {
		if _, err := AbsDir(`C:\src\repo`); err != nil {
			t.Error(err)
		}
	}
}
