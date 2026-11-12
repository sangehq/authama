package editor

import "testing"

// A key root reaches a rename only as one plain directory name.
func TestOnlyOnePlainNameIsAKeyRootPath(t *testing.T) {
	for name, want := range map[string]bool{"keys": true, "profiles": true, "": false, ".": false, "..": false,
		"../etc": false, "a/b": false, "/abs": false} {
		if got := oneName(name); got != want {
			t.Errorf("oneName(%q) = %v, want %v", name, got, want)
		}
	}
}
