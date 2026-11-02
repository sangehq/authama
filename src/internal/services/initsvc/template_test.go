package initsvc

import (
	"bytes"
	"strings"
	"testing"

	"github.com/simtabi/authama/src/internal/services/settings"
)

// The .env authama init seeds is loaded at every start, so the template must
// parse cleanly, set nothing, and use the current variable names.
func TestTheEnvTemplateLoadsCleanly(t *testing.T) {
	pairs, warns := settings.ParseDotenv(bytes.NewReader(defaultEnv))
	if len(warns) > 0 {
		t.Errorf("the template has malformed lines: %v", warns)
	}
	for _, p := range pairs {
		if p.Value != "" {
			t.Errorf("line %d: %s has a value; the template documents shape only", p.Line, p.Key)
		}
		if strings.HasPrefix(p.Key, "SSH_MANAGER_") || strings.HasPrefix(p.Key, "SSHMGR_") {
			t.Errorf("line %d: %s is a pre-0.1.1 name", p.Line, p.Key)
		}
	}
	if len(pairs) == 0 {
		t.Fatal("no assignments found")
	}
}
