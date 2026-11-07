package brand

import "testing"

func TestTheNamesAreSet(t *testing.T) {
	if Name == "" || Command == "" || Summary == "" {
		t.Fatal("a brand constant is empty")
	}
}
