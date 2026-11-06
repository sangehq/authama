package brand

import "testing"

func TestTheNamesAreSetAndDistinct(t *testing.T) {
	if Name == "" || Command == "" || LegacyCommand == "" || Summary == "" {
		t.Fatal("a brand constant is empty")
	}
	if Command == LegacyCommand {
		t.Fatal("the legacy command must differ from the current one")
	}
}
