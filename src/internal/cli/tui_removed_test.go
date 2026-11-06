package cli

import (
	"errors"
	"strings"
	"testing"
)

// The menu is removed, but the name still answers with the way forward, and
// is not advertised.
func TestTheRemovedTUIPointsAtTheWebUI(t *testing.T) {
	root := newRootCmd()
	cmd, _, err := root.Find([]string{"tui"})
	if err != nil || cmd == root || cmd.Name() != "tui" {
		t.Fatalf("`tui` should still resolve to its stub, got %v (%v)", cmd, err)
	}
	if cmd.IsAvailableCommand() {
		t.Error("the stub should be left out of help")
	}
	err = cmd.RunE(cmd, nil)
	if !errors.Is(err, errTUIRemoved) || !strings.Contains(err.Error(), "serve") {
		t.Errorf("`tui` should fail with the pointer to serve, got %v", err)
	}
}
