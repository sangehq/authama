package renderer

import "testing"

// An option typed in between a block's Host line and its comment leaves the
// comment the block's: it is read wherever it sits among the options.
func TestParseManagedKeepsACommentBelowATypedOption(t *testing.T) {
	text := "# Managed by authama - do not edit (run: authama ssh config render)\n\n" +
		"# prod: Cloudflare Access\nHost web web-cf\n    ServerAliveCountMax 4\n    # the web box\n    HostName web.example\n    User deploy\n"
	got := ParseManaged(text)
	if len(got.Blocks) != 1 {
		t.Fatalf("parsed %+v", got)
	}
	b := got.Blocks[0]
	if b.Comment != "the web box" || b.Section != "prod: Cloudflare Access" || len(got.Loose) != 0 || len(b.Options) != 3 {
		t.Fatalf("parsed %+v (loose %q)", b, got.Loose)
	}
}
