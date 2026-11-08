package inventory

import (
	"path/filepath"
	"testing"
)

func TestRetirementsRoundTrip(t *testing.T) {
	path := RetiredPath(filepath.Join(t.TempDir(), "inventory.json"))
	if r, err := LoadRetirements(path); err != nil || len(r) != 0 {
		t.Fatalf("a missing file: %v, %v", r, err)
	}
	r := Retirements{"SHA256:old": {Key: "work/hpc", Successor: "SHA256:new", RetiredOn: "2026-11-08",
		Removed: []string{"b", "a"}, ByHand: map[string]string{"panel": "https://panel.example/keys"}}}
	if err := r.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadRetirements(path)
	if err != nil {
		t.Fatal(err)
	}
	g := got["SHA256:old"]
	if g.Key != "work/hpc" || g.Removed[0] != "a" || g.NotRemoved == nil || g.ByHand["panel"] == "" {
		t.Fatalf("round trip: %+v", g)
	}
}
