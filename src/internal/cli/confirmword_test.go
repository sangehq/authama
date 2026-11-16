package cli

import (
	"errors"
	"strings"
	"testing"
)

// A --confirm word that is not the one asked for says so, and which word was
// wanted, rather than exiting 1 with nothing on the screen (audit B-10).
func TestAWrongConfirmWordSaysWhatWasExpected(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{"reset", "--confirm", "nope"}, `--confirm must be "reset"`},
		{[]string{"history", "reset", "--confirm", "nope"}, `--confirm must be "history"`},
	} {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			editHome(t)
			run(t, "ssh", "profile", "add", "work") // so there is a history to reset
			_, err := runErr(t, c.args...)
			if err == nil {
				t.Fatal("a wrong --confirm word went ahead")
			}
			if errors.Is(err, errAborted) || !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %v, want one saying %s (errAborted prints nothing)", err, c.want)
			}
		})
	}
}
