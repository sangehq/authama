package settings

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// Schema renders the JSON Schema (draft 2020-12) for authama.json from Specs.
// docs/schema/authama.schema.json is this output; a test keeps them equal.
func Schema() ([]byte, error) {
	type obj = map[string]any
	props := obj{
		"$schema": obj{"type": "string", "description": "The schema this file follows (for editors)."},
		"version": obj{"const": Version, "description": "The authama.json format version."},
	}
	for _, sec := range scalarSectionNames() {
		sp := obj{}
		for _, s := range Specs {
			if s.Section != sec {
				continue
			}
			p := obj{"description": s.Doc + " Sets $" + s.Env + "."}
			if s.Default != "" {
				p["description"] = p["description"].(string) + " Default: " + s.Default + "."
			}
			switch s.Kind {
			case KindString, KindPath:
				p["type"], p["minLength"] = "string", 1
			case KindEnum:
				p["type"], p["enum"] = "string", s.Enum
			case KindList:
				item := "(" + strings.Join(s.Enum, "|") + ")"
				p["type"], p["pattern"] = "string", `^ *`+item+` *(, *`+item+` *)*$`
			case KindInt:
				p["type"], p["minimum"] = "integer", s.Min
				if d, err := strconv.Atoi(s.Default); err == nil {
					p["default"] = d
				}
			case KindBool:
				p["type"] = "boolean"
				if s.Default != "" {
					p["default"] = s.Default == "true"
				}
			}
			sp[s.Name] = p
		}
		props[sec] = obj{"type": "object", "additionalProperties": false, "properties": sp}
	}
	for k, v := range structuredSchema() {
		props[k] = v
	}
	doc := obj{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"$id":                  SchemaURL,
		"title":                "authama settings (authama.json)",
		"description":          "User settings for authama. Precedence: command-line flags > environment > <config home>/.env > authama.json > defaults.",
		"type":                 "object",
		"additionalProperties": false,
		"properties":           props,
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // keep "<config home>" and ">" readable
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
