package seal

import (
	"testing"
	"time"

	"github.com/simtabi/authama/src/internal/util/testcred"
)

var (
	here = Binding{User: "alice", UID: "501", Machine: "m-1"}
	when = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
)

func key(purpose string) []byte { return []byte(testcred.Secret(purpose)) }

func files() map[string][]byte {
	return map[string][]byte{"manifest.json": []byte(`{"a":1}`), "tokens.json": nil}
}

func TestAnUntouchedHomeVerifiesClean(t *testing.T) {
	r, err := Make(key("k"), here, files(), when)
	if err != nil {
		t.Fatal(err)
	}
	if rep := Verify(key("k"), here, r, files()); !rep.Clean() {
		t.Fatalf("%+v", rep)
	}
}

func TestEachKindOfChangeIsNamed(t *testing.T) {
	r, _ := Make(key("k"), here, files(), when)
	now := map[string][]byte{"manifest.json": []byte(`{"a":2}`), "tokens.json": []byte("{}"), "inventory.json": []byte("{}")}
	rep := Verify(key("k"), here, r, now)
	want := map[string]Status{"manifest.json": Changed, "tokens.json": Appeared, "inventory.json": Unsealed}
	if !rep.Authentic || !rep.Bound || len(rep.Findings) != len(want) {
		t.Fatalf("%+v", rep)
	}
	for _, f := range rep.Findings {
		if want[f.Name] != f.Status {
			t.Errorf("%s: %s, want %s", f.Name, f.Status, want[f.Name])
		}
	}
	gone := Verify(key("k"), here, r, map[string][]byte{"manifest.json": nil})
	if len(gone.Findings) != 1 || gone.Findings[0].Status != Missing {
		t.Errorf("deleted file: %+v", gone)
	}
}

func TestAnotherAccountOrMachineIsNotBound(t *testing.T) {
	r, _ := Make(key("k"), here, files(), when)
	for _, b := range []Binding{{User: "bob", UID: "501", Machine: "m-1"}, {User: "alice", UID: "501", Machine: "m-2"}} {
		if rep := Verify(key("k"), b, r, files()); !rep.Authentic || rep.Bound || rep.Clean() {
			t.Errorf("%+v verified as bound: %+v", b, rep)
		}
	}
}

// Editing the record - dropping a file from it, or using another key - fails
// the record's own MAC, so a seal cannot be trimmed to hide a change.
func TestATamperedRecordIsNotAuthentic(t *testing.T) {
	r, _ := Make(key("k"), here, files(), when)
	trimmed := r
	trimmed.Files = map[string]string{"manifest.json": r.Files["manifest.json"]}
	if Verify(key("k"), here, trimmed, files()).Authentic {
		t.Error("a record with an entry removed is authentic")
	}
	if Verify(key("other"), here, r, files()).Authentic {
		t.Error("a record verified under another key")
	}
	if Verify(nil, here, r, files()).Authentic {
		t.Error("a record verified with no key")
	}
	if _, err := Make(nil, here, files(), when); err == nil {
		t.Error("Make sealed with no key")
	}
}
