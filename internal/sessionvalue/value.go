// Package sessionvalue owns the canonical, detached representation used by
// Gonex session implementations. It is internal so the representation can
// evolve without expanding the public framework API.
package sessionvalue

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// Normalize converts value into a detached JSON-safe representation. Numbers
// use json.Number so integer precision is not lost merely by crossing a session
// boundary. Unsupported values, cycles, NaN, and infinities are rejected by
// encoding/json rather than retained by reference.
func Normalize(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("session value is not JSON-safe: %w", err)
	}
	var normalized any
	if err := Decode(data, &normalized); err != nil {
		return nil, fmt.Errorf("decode normalized session value: %w", err)
	}
	return normalized, nil
}

// NormalizeMap converts a session value map into the canonical detached form.
func NormalizeMap(values map[string]any) (map[string]any, error) {
	if values == nil {
		return map[string]any{}, nil
	}
	normalized, err := Normalize(values)
	if err != nil {
		return nil, err
	}
	result, ok := normalized.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("normalized session values are not an object")
	}
	return result, nil
}

// Decode performs one strict JSON decode while preserving numbers as
// json.Number. Trailing JSON values are rejected.
func Decode(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

// Clone returns a detached copy of a canonical session value without another
// JSON encode/decode cycle. Normalize should be used first at trust/ownership
// boundaries.
func Clone(value any) any {
	switch value := value.(type) {
	case map[string]any:
		return CloneMap(value)
	case []any:
		clone := make([]any, len(value))
		for index := range value {
			clone[index] = Clone(value[index])
		}
		return clone
	default:
		// Canonical scalar values (nil, bool, string, json.Number) are immutable.
		return value
	}
}

// CloneMap returns a detached copy of a canonical session map.
func CloneMap(values map[string]any) map[string]any {
	clone := make(map[string]any, len(values))
	for key, value := range values {
		clone[key] = Clone(value)
	}
	return clone
}
