package selection

import (
	"errors"
	"slices"
	"strings"
)

// Comparison is two results side by side: the columns they share, which the
// rows are matched on, and the rows only one of them holds.
type Comparison struct {
	// On are the shared columns, in the first result's order.
	On    []string   `json:"on"`
	OnlyA [][]string `json:"only_a"`
	OnlyB [][]string `json:"only_b"`
	Both  int        `json:"both"`
	// KeptA and KeptB are each result's rows.
	KeptA int `json:"kept_a"`
	KeptB int `json:"kept_b"`
}

// ErrNothingShared is two results with no column in common.
var ErrNothingShared = errors.New("the two selections have no column in common to compare on")

// Compare matches a's and b's rows on the columns they share. A row is in
// both when its shared cells are; each side's duplicates count once.
func Compare(a, b Result) (Comparison, error) {
	var on []string
	for _, c := range a.Columns {
		if slices.Contains(b.Columns, c) && !slices.Contains(on, c) {
			on = append(on, c)
		}
	}
	if len(on) == 0 {
		return Comparison{}, ErrNothingShared
	}
	project := func(r Result) ([]string, map[string][]string) {
		idx := make([]int, len(on))
		for i, c := range on {
			idx[i] = slices.Index(r.Columns, c)
		}
		var order []string
		rows := map[string][]string{}
		for _, row := range r.Rows {
			cells := make([]string, len(on))
			for i, j := range idx {
				cells[i] = Cell(row[j])
			}
			k := strings.Join(cells, "\x00")
			if _, ok := rows[k]; !ok {
				order = append(order, k)
				rows[k] = cells
			}
		}
		return order, rows
	}
	ka, ra := project(a)
	kb, rb := project(b)
	cmp := Comparison{On: on, OnlyA: [][]string{}, OnlyB: [][]string{}, KeptA: len(a.Rows), KeptB: len(b.Rows)}
	for _, k := range ka {
		if _, ok := rb[k]; ok {
			cmp.Both++
		} else {
			cmp.OnlyA = append(cmp.OnlyA, ra[k])
		}
	}
	for _, k := range kb {
		if _, ok := ra[k]; !ok {
			cmp.OnlyB = append(cmp.OnlyB, rb[k])
		}
	}
	return cmp, nil
}
