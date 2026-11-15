package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/sangehq/authama/src/internal/util/fs"
)

// Set writes one setting into the authama.json at path, creating the file
// (as Starter) when there is none and keeping every other setting as it was.
// The value is given as it would be typed - true, 10, a path - and is checked
// against the setting's kind before anything is written; the whole file is
// checked again after, so Set never leaves a file Parse refuses.
func Set(path, key, value string) error {
	spec, ok := Find(key)
	if !ok {
		return fmt.Errorf("%s: unknown setting (authama settings show --effective lists them)", key)
	}
	raw, err := spec.literal(value)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	return edit(path, spec, &raw)
}

// Unset takes one setting out of the authama.json at path, so it goes back
// to its default; every other setting is kept. A setting the file does not
// hold is not an error.
func Unset(path, key string) error {
	spec, ok := Find(key)
	if !ok {
		return fmt.Errorf("%s: unknown setting (authama settings show --effective lists them)", key)
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return edit(path, spec, nil)
}

// FileValue is the value authama.json at path holds for key, as the
// environment variable would carry it, and whether it holds one.
func FileValue(path, key string) (string, bool) {
	f, err := LoadFile(path)
	if err != nil || f == nil {
		return "", false
	}
	v, ok := f.Values[key]
	return v, ok
}

// edit writes raw as spec's value into the file at path, or with raw nil
// takes the setting out (and its section, once empty).
func edit(path string, spec Spec, raw *json.RawMessage) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		b, err = Starter(), nil
	}
	if err != nil {
		return err
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		return fmt.Errorf("%s is not a JSON object: %w", path, err)
	}
	var sec map[string]json.RawMessage
	if s, ok := top[spec.Section]; ok {
		if err := json.Unmarshal(s, &sec); err != nil {
			return fmt.Errorf("%s: %s must be an object", path, spec.Section)
		}
	}
	if sec == nil {
		sec = map[string]json.RawMessage{}
	}
	if raw == nil {
		delete(sec, spec.Name)
	} else {
		sec[spec.Name] = *raw
	}
	if len(sec) == 0 {
		delete(top, spec.Section)
	} else if top[spec.Section], err = json.Marshal(sec); err != nil {
		return err
	}
	out, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')
	if _, err := Parse(path, out); err != nil {
		return err
	}
	return fs.WriteTextAtomic(path, string(out), 0o600)
}

// literal is value as the JSON a setting of this kind holds.
func (s Spec) literal(value string) (json.RawMessage, error) {
	value = strings.TrimSpace(value)
	var raw json.RawMessage
	switch s.Kind {
	case KindInt:
		if _, err := strconv.Atoi(value); err != nil {
			return nil, fmt.Errorf("must be an integer, got %q", value)
		}
		raw = json.RawMessage(value)
	case KindBool:
		b, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("must be true or false, got %q", value)
		}
		raw = json.RawMessage(strconv.FormatBool(b))
	default:
		q, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		raw = q
	}
	if _, err := s.convert(bytes.TrimSpace(raw)); err != nil {
		return nil, err
	}
	return raw, nil
}
