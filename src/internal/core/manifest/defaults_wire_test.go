package manifest

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// Defaults is written by a hand-listed wire struct, so a field added to
// Defaults and not to the wire is silently dropped on every save. Each field,
// set to something non-zero, must come back out of MarshalJSON under its tag.
func TestEveryDefaultsFieldSurvivesSaving(t *testing.T) {
	var d Defaults
	v := reflect.ValueOf(&d).Elem()
	typ := v.Type()
	var tags []string
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		tag := strings.Split(f.Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			continue
		}
		tags = append(tags, tag)
		fv := v.Field(i)
		switch fv.Kind() {
		case reflect.String:
			fv.SetString("x")
		case reflect.Int:
			fv.SetInt(1)
		case reflect.Slice:
			fv.Set(reflect.MakeSlice(fv.Type(), 1, 1))
		case reflect.Pointer:
			fv.Set(reflect.New(fv.Type().Elem()))
		}
	}
	if len(tags) < 10 {
		t.Fatalf("inspected %d Defaults fields; the reflection walk has stopped matching", len(tags))
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	for _, tag := range tags {
		if _, ok := out[tag]; !ok {
			t.Errorf("defaults.%s is set but MarshalJSON does not write it", tag)
		}
	}
}
