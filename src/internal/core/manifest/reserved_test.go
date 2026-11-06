package manifest

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReservedKeysRoundTripAndValidate(t *testing.T) {
	m := &Manifest{Version: schemaVersion, Defaults: newDefaults(), Profiles: map[string]Profile{}}
	before, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(before), "reserved_keys") {
		t.Fatalf("an empty manifest should not mention reserved_keys: %s", before)
	}
	Reserve(m, "docker-tunnel", ReservedKey{For: "devbox", Note: "Docker VM", Since: "2026-11-02"})
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var back Manifest
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	if back.ReservedKeys["docker-tunnel"].Note != "Docker VM" {
		t.Fatalf("lost the reservation: %s", b)
	}
	if !Unreserve(&back, "docker-tunnel") || back.ReservedKeys != nil {
		t.Error("unreserve")
	}
	for _, bad := range []string{"x.pub", "config", "known_hosts", "../x", "-x", "a/b"} {
		if ValidReservedKeyName(bad) == nil {
			t.Errorf("%q should be rejected", bad)
		}
	}
	Reserve(m, "ok", ReservedKey{Note: "bad\nnote"})
	if m.Validate() == nil {
		t.Error("a control character in a note should be rejected")
	}
}
