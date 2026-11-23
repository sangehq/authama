package change

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sangehq/authama/src/internal/services/history"
	"github.com/sangehq/authama/src/internal/services/structure"
)

// A structure move previews as a private key removed from one path and
// created at another - the copy holds a placeholder, never the key - beside
// the manifest diff, and the real home is untouched until apply.
func TestStructurePlanPreviewsTheMoveThenApplies(t *testing.T) {
	p := home(t)
	loose := filepath.Join(p.SSHDir, "id_laptop")
	if err := os.WriteFile(loose, []byte(privateMarker+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(loose+".pub", []byte("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIB/22QXk5vTmOo31cReZdmvraArThvJ6FLu4g9t8PFIF me@work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	moves, err := structure.ParsePlan("key ~/.ssh/id_laptop profile work")
	if err != nil {
		t.Fatal(err)
	}
	ch := StructurePlan(moves)
	plan, err := Preview(p, ch)
	if err != nil || plan.Err != "" {
		t.Fatalf("preview: %v %s", err, plan.Err)
	}
	hidden := strings.Join(plan.Hidden, "\n")
	if !strings.Contains(hidden, "id_laptop") || !strings.Contains(hidden, "profiles/work/id_laptop") {
		t.Errorf("hidden = %v", plan.Hidden)
	}
	var manifestDiff bool
	for _, f := range plan.Files {
		if f.Path == "config/manifest.json" {
			manifestDiff = true
		}
	}
	if !manifestDiff {
		t.Errorf("no manifest diff: %+v", plan.Files)
	}
	if _, err := os.Stat(loose); err != nil {
		t.Fatal("preview moved the real key")
	}
	if b, _ := os.ReadFile(loose); !strings.Contains(string(b), privateMarker) {
		t.Fatal("preview changed the real key")
	}
	if _, _, err := Apply(p, ch, plan.Hash, 10, history.SourceWeb); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(p.SSHDir, "profiles", "work", "id_laptop"))
	if err != nil || !strings.Contains(string(b), privateMarker) {
		t.Fatalf("moved key: %q %v", b, err)
	}
}
