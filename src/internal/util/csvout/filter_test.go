package csvout

import (
	"reflect"
	"testing"
)

func sample() Table {
	return Table{Header: []string{"alias", "host", "user"}, Rows: [][]string{
		{"gh", "github.com", "git"}, {"db", "db.internal", "deploy"}, {"web", "WEB.example", "deploy"},
	}}
}

func TestDropRemovesColumnsAndIgnoresUnknownOnes(t *testing.T) {
	got := sample().Drop("host", "nope")
	if !reflect.DeepEqual(got.Header, []string{"alias", "user"}) || !reflect.DeepEqual(got.Rows[1], []string{"db", "deploy"}) {
		t.Fatalf("%+v", got)
	}
	if !reflect.DeepEqual(sample().Drop(), sample()) {
		t.Fatal("Drop() changed the table")
	}
}

func TestWithoutDropsMatchingRowsIgnoringCase(t *testing.T) {
	got := sample().Without("web")
	if len(got.Rows) != 2 || got.Rows[1][0] != "db" {
		t.Fatalf("%+v", got)
	}
	if len(sample().Without("  ").Rows) != 3 {
		t.Fatal("an empty filter dropped rows")
	}
}

func TestTheFiltersChainAndParseRoundTrips(t *testing.T) {
	got := sample().Without("github").Drop("user").Head(1)
	if !reflect.DeepEqual(got, Table{Header: []string{"alias", "host"}, Rows: [][]string{{"db", "db.internal"}}}) {
		t.Fatalf("%+v", got)
	}
	p, err := Parse("a,b\n1,2\n3,\"x,y\"\n")
	if err != nil || !reflect.DeepEqual(p, Table{Header: []string{"a", "b"}, Rows: [][]string{{"1", "2"}, {"3", "x,y"}}}) {
		t.Fatalf("%+v %v", p, err)
	}
}
