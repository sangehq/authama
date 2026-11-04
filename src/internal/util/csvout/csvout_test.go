package csvout

import (
	"strings"
	"testing"
)

// A table is header-first RFC 4180, quoted where a cell needs it, and no cell
// can start a spreadsheet formula.
func TestWriteQuotesAndNeutralisesFormulas(t *testing.T) {
	var b strings.Builder
	err := Write(&b, Table{Header: []string{"alias", "note"}, Rows: [][]string{
		{"web1", "a, b"},
		{"=HYPERLINK(\"http://x\")", "+1"},
		{"-rm", "@SUM(A1)"},
		{"line\nbreak", `quote "x"`},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := "alias,note\n" +
		"web1,\"a, b\"\n" +
		"\"'=HYPERLINK(\"\"http://x\"\")\",'+1\n" +
		"'-rm,'@SUM(A1)\n" +
		"\"line\nbreak\",\"quote \"\"x\"\"\"\n"
	if b.String() != want {
		t.Fatalf("got\n%s\nwant\n%s", b.String(), want)
	}
}
