package providers

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
)

// Redirect is an entry of a providers.json that would send a token somewhere
// this build's own catalog would not: it differs from the built-in entry of
// the same name, or adds a name that sends one.
type Redirect struct {
	Name     string // the provider's name
	TokenEnv string // the variable whose value it sends, or "" for the built-in default
	Dest     string // where it sends it: api, rest.base_url or host
}

// RedirectsIn lists, sorted by name, the entries of doc (a providers.json
// document) that a person should see before importing it: one that replaces a
// built-in provider, or one that differs from this build's catalog and names a
// token to send. A document does not prove who made it, and a provider entry
// decides which host receives the user's token.
func RedirectsIn(doc []byte) ([]Redirect, error) {
	var top struct {
		Providers map[string]json.RawMessage `json:"providers"`
	}
	if err := json.Unmarshal(doc, &top); err != nil {
		return nil, fmt.Errorf("providers.json does not parse: %w", err)
	}
	defaults := catalogProviders("")
	var out []Redirect
	for name, raw := range top.Providers {
		if def, ok := defaults[name]; ok && sameJSON(def, raw) {
			continue
		}
		var e catalogEntry
		if json.Unmarshal(raw, &e) != nil {
			continue // AllSpecs skips such an entry too
		}
		_, builtin := builtinSpecs[name]
		_, known := defaults[name]
		if !builtin && !known && e.TokenEnv == "" {
			continue // a new provider that sends no token
		}
		dest := e.API
		if dest == "" {
			if b, ok := e.Rest["base_url"].(string); ok {
				dest = b
			}
		}
		if dest == "" {
			dest = e.Host
		}
		out = append(out, Redirect{Name: name, TokenEnv: e.TokenEnv, Dest: dest})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func sameJSON(a, b json.RawMessage) bool {
	var x, y any
	return json.Unmarshal(a, &x) == nil && json.Unmarshal(b, &y) == nil && reflect.DeepEqual(x, y)
}
