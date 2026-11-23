package tokens

import "testing"

// pyRepr quotes as Python's repr does, and a quote inside never ends the
// quoted text early.
func TestPyReprQuotesAsPython(t *testing.T) {
	for in, want := range map[string]string{
		`plain`:      `'plain'`,
		`it's`:       `"it's"`,
		`a\b's`:      `"a\\b's"`,
		`both'"`:     `'both\'"'`,
		`back\slash`: `'back\\slash'`,
	} {
		if got := pyRepr(in); got != want {
			t.Errorf("pyRepr(%q) = %s, want %s", in, got, want)
		}
	}
}
