package inventory

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// inventory.json's schema lists exactly the fields the code writes, as the
// manifest's does (TestSchemaListsEveryManifestField): nothing guarded it, so
// a field added to KeyRecord would be refused by the schema's
// additionalProperties:false without anyone noticing (review R-INC-01).
func TestSchemaListsEveryInventoryField(t *testing.T) {
	raw, err := os.ReadFile("../../../config/schema/inventory.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	props := func(path ...string) []string {
		var node any = s
		for _, p := range path {
			node = node.(map[string]any)[p]
		}
		var keys []string
		for k := range node.(map[string]any)["properties"].(map[string]any) {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys
	}
	tags := func(v any) []string {
		var keys []string
		typ := reflect.TypeOf(v)
		for i := 0; i < typ.NumField(); i++ {
			if tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]; tag != "" && tag != "-" {
				keys = append(keys, tag)
			}
		}
		sort.Strings(keys)
		return keys
	}
	checks := []struct {
		name         string
		schema, code []string
	}{
		{"inventory", props(), tags(Inventory{})},
		{"keys.*", props("properties", "keys", "additionalProperties"), tags(KeyRecord{})},
		{"keys.*.deployments[]", props("properties", "keys", "additionalProperties", "properties", "deployments", "items"), tags(Deployment{})},
	}
	if len(checks) < 3 {
		t.Fatal("inspected fewer types than exist")
	}
	for _, c := range checks {
		if len(c.code) == 0 {
			t.Fatalf("%s: no JSON fields found in the code; the reflection is not reading the type", c.name)
		}
		if !reflect.DeepEqual(c.schema, c.code) {
			t.Errorf("%s: schema %v, code %v", c.name, c.schema, c.code)
		}
	}
}
