package selection

import (
	"sort"

	"github.com/sangehq/authama/src/internal/services/dataset"
)

// Presets are the built-in selections, one per list the pages download, by
// the list's name: each is its grain with the default columns, so
// /exports/run?preset=hosts is the hosts CSV.
var Presets = map[string]Selection{
	"hosts":       {Version: Version, Grain: GrainHost},
	"keys":        {Version: Version, Grain: GrainKey},
	"profiles":    {Version: Version, Grain: GrainProfile},
	"tokens":      {Version: Version, Grain: GrainToken},
	"history":     {Version: Version, Grain: GrainOp},
	"events":      {Version: Version, Grain: GrainEvent},
	"snapshots":   {Version: Version, Grain: GrainSnapshot},
	"deployments": {Version: Version, Grain: GrainDeployment},
}

// Parts is the dataset parts a grain needs, so a selection reads only what
// it uses: profiles, keys and hosts are always read.
func Parts(grain string) []string {
	switch grain {
	case GrainToken:
		return []string{dataset.PartTokens}
	case GrainOp:
		return []string{dataset.PartOps}
	case GrainEvent:
		return []string{dataset.PartEvents}
	case GrainSnapshot:
		return []string{dataset.PartSnapshots}
	case GrainDeployment:
		return []string{dataset.PartDeployments}
	}
	return []string{dataset.PartCore}
}

// FacetValue is one value a facet can take, and how many of the grain's rows
// have it.
type FacetValue struct {
	Value   string `json:"value"`
	Profile string `json:"profile,omitempty"` // a key's profile, so a page can group keys
	Rows    int    `json:"rows"`
}

// facetFields are the facets and the field each reads; a list field (tags)
// counts each of its values.
var facetFields = map[string]string{
	"profile": "profile", "env": "env", "role": "role", "tag": "tags", "status": "status",
	"target": "target", "kind": "kind", "area": "area", "source": "source",
}

// Facets is every value each facet can take over a grain's rows, before any
// filter, in first-seen order: what a page offers to tick. Keys come with
// their profile; a facet no row has a value for is left out.
func Facets(ds *dataset.Dataset, grain string) (map[string][]FacetValue, error) {
	rows, _, err := rowsOf(ds, grain)
	if err != nil {
		return nil, err
	}
	out := map[string][]FacetValue{}
	add := func(facet, value, profile string) {
		if value == "" {
			return
		}
		for i, v := range out[facet] {
			if v.Value == value {
				out[facet][i].Rows++
				return
			}
		}
		out[facet] = append(out[facet], FacetValue{Value: value, Profile: profile, Rows: 1})
	}
	names := make([]string, 0, len(facetFields))
	for n := range facetFields {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, r := range rows {
		for _, facet := range names {
			switch v := r.f[facetFields[facet]].(type) {
			case string:
				add(facet, v, "")
			case []string:
				for _, x := range v {
					add(facet, x, "")
				}
			}
		}
		for _, k := range r.keys {
			add("key", k.Name, k.Profile)
		}
	}
	return out, nil
}
