package tokens

import (
	"strings"
	"testing"
)

// The shipped examples are what a user starts from, so they must pass; a
// literal where a name belongs, or a credential header without ${VAR}, must not.
func TestCheckNamesOnly(t *testing.T) {
	if err := CheckNamesOnly(AllTargetsTemplate()); err != nil {
		t.Fatalf("the all-targets example fails: %v", err)
	}
	for _, tgt := range Targets() {
		c, err := TargetTemplate(tgt)
		if err != nil {
			t.Fatal(err)
		}
		if err := CheckNamesOnly(c.JSON()); err != nil {
			t.Errorf("the %s example fails: %v", tgt, err)
		}
	}
	// A value where a variable name belongs: a literal, not a name.
	literal := "x" + strings.Repeat("Z9", 8) + "-not-a-name"
	bad := map[string]string{
		"a value in pass_env": `{"git": {"github.com": {"user_env": "GH_USER", "pass_env": "` + literal + `"}}}`,
		"a number in env":     `{"npm": {"registry.npmjs.org": {"env": 42}}}`,
		"a bare auth header":  `{"composer": {"h.example": {"service": "generic-headers", "headers": ["API-TOKEN: ` + literal + `"]}}}`,
		"not an object":       `[1, 2]`,
		"not JSON":            `{`,
	}
	for name, text := range bad {
		if err := CheckNamesOnly(text); err == nil {
			t.Errorf("%s passed", name)
		}
	}
	// A real token is a valid POSIX name; the check must not take it for one.
	for _, tok := range []string{"ghp_" + strings.Repeat("aB3", 12), strings.Repeat("Ab", 10), "GHP_" + strings.Repeat("X9", 18), strings.Repeat("A", 49)} {
		if err := CheckNamesOnly(`{"npm": {"registry.npmjs.org": {"env": "` + tok + `"}}}`); err == nil {
			t.Errorf("%.12s… passed as a variable name", tok)
		}
	}
	ok := `{"composer": {"h.example": {"service": "generic-headers", "headers": ["API-TOKEN: ${MY_TOKEN}", "X-Org: acme"]}}}`
	if err := CheckNamesOnly(ok); err != nil {
		t.Errorf("a ${VAR} header and a plain one failed: %v", err)
	}
	if err := CheckNamesOnly("{" + strings.Repeat(" ", MaxConfigBytes) + "}"); err == nil {
		t.Error("an oversized config passed")
	}
}
