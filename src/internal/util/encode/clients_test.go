package encode

import "testing"

// Each client's escaping, for its own hazard: a value must stay a value.
// These moved here with the writers from core/renderer.
func TestClientValuesAreEscapedForEachClient(t *testing.T) {
	if xmlText(`a<b&"c`) != "a&lt;b&amp;&#34;c" {
		t.Errorf("SecureCRT XML escaping: %q", xmlText(`a<b&"c`))
	}
	if got := mobaField("a%b#c=d\ne"); got != "a_b_c-d e" {
		t.Errorf("MobaXterm: a separator in a value shifts the fields: %q", got)
	}
	if puttyName(`a b\c*%`) != `a%20b%5Cc%2A%25` {
		t.Errorf("PuTTY session name escaping: %s", puttyName(`a b\c*%`))
	}
	if remminaValue(" a\\b\n") != `\sa\\b\n` {
		t.Errorf("Remmina escaping: %q", remminaValue(" a\\b\n"))
	}
	if cell("=1+1") != "'=1+1" || termiusTags([]string{"a,b", "-c"}) != "a b,'-c" {
		t.Errorf("formula cells: %q %q", cell("=1+1"), termiusTags([]string{"a,b", "-c"}))
	}
}
