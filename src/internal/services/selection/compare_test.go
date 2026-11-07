package selection

import (
	"errors"
	"reflect"
	"testing"
)

func TestCompareMatchesOnTheSharedColumns(t *testing.T) {
	ds, _, _ := build(t)
	run := func(s Selection) Result {
		t.Helper()
		s.Version, s.Grain = Version, GrainHost
		r, err := Run(ds, s)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	simtabi := run(Selection{Where: Where{Profile: []string{"simtabi"}}, Columns: []Column{{Field: "alias"}, {Field: "user"}}, Sort: []Sort{{Field: "alias"}}})
	web := run(Selection{Where: Where{Tag: []string{"web"}}, Columns: []Column{{Field: "alias"}, {Field: "profile"}}, Sort: []Sort{{Field: "alias"}}})
	c, err := Compare(simtabi, web)
	if err != nil {
		t.Fatal(err)
	}
	want := Comparison{On: []string{"alias"}, Both: 1, KeptA: 3, KeptB: 2,
		OnlyA: [][]string{{"oribi-web"}, {"simbi-prd-db-00"}},
		OnlyB: [][]string{{"dani-stg-web-00"}}}
	if !reflect.DeepEqual(c, want) {
		t.Errorf("compare =\n%+v\nwant\n%+v", c, want)
	}
	other := run(Selection{Columns: []Column{{Field: "port"}}})
	if _, err := Compare(simtabi, other); !errors.Is(err, ErrNothingShared) {
		t.Errorf("no shared column: %v", err)
	}
	// Duplicates on the shared columns count once.
	users := run(Selection{Columns: []Column{{Field: "user"}}})
	c, _ = Compare(users, users)
	if c.Both != 2 || len(c.OnlyA) != 0 || c.KeptA != 5 {
		t.Errorf("self compare = %+v", c)
	}
}
