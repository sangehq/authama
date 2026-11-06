package tokens

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// The auth files this package edits are shared with the tools that own them:
// composer, docker and the people who hand-edit both. A merge that reordered
// every key would show up as a whole-file diff for a one-host change, so JSON is
// read into an order-preserving map and written back in the order it was read,
// in the same layout the Python tool this replaced produced (two-space indent,
// ASCII-escaped), so a file it wrote and a file this writes are byte-identical.

// object is a JSON object that remembers its key order.
type object struct {
	keys []string
	vals map[string]any
}

func newObject() *object { return &object{vals: map[string]any{}} }

// set inserts or replaces a key; a replaced key keeps its position.
func (o *object) set(k string, v any) {
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func (o *object) get(k string) (any, bool) {
	v, ok := o.vals[k]
	return v, ok
}

func (o *object) del(k string) {
	if _, ok := o.vals[k]; !ok {
		return
	}
	delete(o.vals, k)
	for i, key := range o.keys {
		if key == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			break
		}
	}
}

func (o *object) clone() *object {
	c := newObject()
	for _, k := range o.keys {
		c.set(k, o.vals[k])
	}
	return c
}

// errNotJSON wraps a parse failure, so callers can tell a malformed file from
// an unreadable one.
var errNotJSON = errors.New("not valid JSON")

// decodeOrdered parses text into *object / []any / string / json.Number /
// bool / nil, keeping object key order.
func decodeOrdered(text string) (any, error) {
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	v, err := decodeValue(dec)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errNotJSON, err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing data", errNotJSON)
	}
	return v, nil
}

func decodeValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			o := newObject()
			for dec.More() {
				kt, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k, _ := kt.(string)
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				o.set(k, v)
			}
			_, err := dec.Token()
			return o, err
		case '[':
			arr := []any{}
			for dec.More() {
				v, err := decodeValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
			_, err := dec.Token()
			return arr, err
		}
		return nil, fmt.Errorf("unexpected %v", t)
	default:
		return t, nil
	}
}

// encodeIndented renders v the way Python's json.dumps(v, indent=2) does.
func encodeIndented(v any) string {
	var b bytes.Buffer
	writeValue(&b, v, 0, true)
	return b.String()
}

// encodeCompact renders v as json.dumps(v, separators=(",", ":")).
func encodeCompact(v any) string {
	var b bytes.Buffer
	writeValue(&b, v, 0, false)
	return b.String()
}

func writeValue(b *bytes.Buffer, v any, depth int, indent bool) {
	nl := func(d int) {
		if indent {
			b.WriteByte('\n')
			b.WriteString(strings.Repeat("  ", d))
		}
	}
	sep := ":"
	if indent {
		sep = ": "
	}
	switch t := v.(type) {
	case *object:
		if len(t.keys) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteByte('{')
		for i, k := range t.keys {
			if i > 0 {
				b.WriteByte(',')
			}
			nl(depth + 1)
			writeString(b, k)
			b.WriteString(sep)
			writeValue(b, t.vals[k], depth+1, indent)
		}
		nl(depth)
		b.WriteByte('}')
	case []any:
		if len(t) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				b.WriteByte(',')
			}
			nl(depth + 1)
			writeValue(b, e, depth+1, indent)
		}
		nl(depth)
		b.WriteByte(']')
	case []string:
		arr := make([]any, len(t))
		for i, s := range t {
			arr[i] = s
		}
		writeValue(b, arr, depth, indent)
	case string:
		writeString(b, t)
	case json.Number:
		b.WriteString(t.String())
	case bool:
		b.WriteString(strconv.FormatBool(t))
	case int:
		b.WriteString(strconv.Itoa(t))
	case nil:
		b.WriteString("null")
	default:
		writeString(b, fmt.Sprint(t))
	}
}

// writeString escapes as Python's json module does with ensure_ascii=True:
// control characters and everything past ASCII become \u escapes (astral
// characters as a surrogate pair), and DEL stays literal.
func writeString(b *bytes.Buffer, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r < 0x20 || (r > 0x7f && r <= 0xffff):
			fmt.Fprintf(b, `\u%04x`, r)
		case r > 0xffff:
			r -= 0x10000
			fmt.Fprintf(b, `\u%04x\u%04x`, 0xd800+(r>>10)&0x3ff, 0xdc00+r&0x3ff)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
}
