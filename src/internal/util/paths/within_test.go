package paths

import (
	"path/filepath"
	"testing"
)

// within is a containment check on two computed paths: a directory whose
// name only starts with ".." is inside, a step up is not.
func TestWithinTellsAStepUpFromANameStartingWithDots(t *testing.T) {
	root := filepath.FromSlash("/home/me")
	for dest, want := range map[string]bool{
		"/home/me":          true,
		"/home/me/.ssh":     true,
		"/home/me/..foo":    true,
		"/home/me/../other": false,
		"/home":             false,
		"/elsewhere":        false,
	} {
		if got := within(root, filepath.FromSlash(dest)); got != want {
			t.Errorf("within(%s, %s) = %v, want %v", root, dest, got, want)
		}
	}
}
