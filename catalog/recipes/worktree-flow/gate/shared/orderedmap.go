package shared

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// OrderedMap is a JSON object that keeps insertion order in both directions.
// It is the ordered container the merge-config decision core and the
// recipe-config writer share: Python dicts are insertion-ordered and both
// references rely on that order (schema defaults in field order, manifest
// pairs in manifest order, object keys on the wire), so no ordered surface may
// cross as a Go map.
//
// Moved out of the gate main package by GO-07 S13 PR2 so the gate binary's
// --plan-merge-config and --write-recipe-config surfaces and the root native
// sync authority share one container. The accessors are exported for the
// callers; the moved merge-config code keeps its original behavior byte for
// byte.
type OrderedMap struct {
	keys   []string
	values map[string]any
}

// NewOrderedMap returns an empty ordered object.
func NewOrderedMap() *OrderedMap {
	return &OrderedMap{values: map[string]any{}}
}

// Keys returns the keys in insertion order. The result aliases the map's own
// storage and is not to be mutated by callers.
func (m *OrderedMap) Keys() []string {
	return m.keys
}

// Values returns the value map. It aliases the map's own storage and is not to
// be mutated by callers.
func (m *OrderedMap) Values() map[string]any {
	return m.values
}

// Set stores key, recording its position on first insertion. Re-setting an
// existing key overwrites the value and keeps the original position, matching
// Python's dict assignment.
func (m *OrderedMap) Set(key string, value any) {
	if _, exists := m.values[key]; !exists {
		m.keys = append(m.keys, key)
	}
	m.values[key] = value
}

// Get returns the value stored under key.
func (m *OrderedMap) Get(key string) (any, bool) {
	value, ok := m.values[key]
	return value, ok
}

// MarshalJSON renders the object with its keys in insertion order.
func (m *OrderedMap) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range m.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyJSON, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		buf.Write(keyJSON)
		buf.WriteByte(':')
		valueJSON, err := json.Marshal(m.values[key])
		if err != nil {
			return nil, err
		}
		buf.Write(valueJSON)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// UnmarshalJSON decodes one JSON object preserving its key order.
func (m *OrderedMap) UnmarshalJSON(data []byte) error {
	value, err := DecodeOrdered(data)
	if err != nil {
		return err
	}
	decoded, ok := value.(*OrderedMap)
	if !ok {
		return fmt.Errorf("expected a JSON object")
	}
	*m = *decoded
	return nil
}

// DecodeOrdered decodes one JSON value preserving object key order. Objects
// become *OrderedMap, arrays []any, numbers json.Number (so int/float
// distinction survives), and strings/bools/null stay as-is.
func DecodeOrdered(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	value, err := decodeOrderedValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, fmt.Errorf("unexpected trailing data")
	}
	return value, nil
}

func decodeOrderedValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	return decodeOrderedFromToken(dec, tok)
}

func decodeOrderedFromToken(dec *json.Decoder, tok json.Token) (any, error) {
	if delim, ok := tok.(json.Delim); ok {
		switch delim {
		case '{':
			m := NewOrderedMap()
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyTok.(string)
				if !ok {
					return nil, fmt.Errorf("invalid object key token")
				}
				value, err := decodeOrderedValue(dec)
				if err != nil {
					return nil, err
				}
				m.Set(key, value)
			}
			if _, err := dec.Token(); err != nil { // closing '}'
				return nil, err
			}
			return m, nil
		case '[':
			arr := []any{}
			for dec.More() {
				value, err := decodeOrderedValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, value)
			}
			if _, err := dec.Token(); err != nil { // closing ']'
				return nil, err
			}
			return arr, nil
		default:
			return nil, fmt.Errorf("unexpected delimiter %v", delim)
		}
	}
	return tok, nil // string, json.Number, bool, or null (nil)
}
