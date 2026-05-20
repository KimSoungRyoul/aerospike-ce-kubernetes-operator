package utils

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// IntFromAny extracts an int from a value that may be int, int64, or float64.
// Returns the fallback value if conversion is not possible.
func IntFromAny(v any, fallback int) int {
	switch p := v.(type) {
	case int:
		return p
	case int64:
		return int(p)
	case float64:
		return int(p)
	default:
		return fallback
	}
}

// ShortSHA256 computes a deterministic short SHA256 hash (first 8 bytes, hex-encoded)
// of the given value by JSON-marshaling it. Returns an empty string if marshaling fails.
func ShortSHA256(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(data)
	return fmt.Sprintf("%x", h[:8])
}

// DeepMerge merges override into base and returns the result.
// Override values take precedence. Nested maps are merged recursively.
// Neither base nor override is modified, and the returned map shares no
// mutable state with either input: nested maps and slices are deep-copied,
// so a later mutation of the result cannot reach back into the source CR
// spec (the common caller passes cluster/rack aerospikeConfig maps).
func DeepMerge(base, override map[string]any) map[string]any {
	result := make(map[string]any, len(base))
	for k, v := range base {
		result[k] = deepCopyValue(v)
	}

	for k, overrideVal := range override {
		baseVal, exists := result[k]
		if !exists {
			result[k] = deepCopyValue(overrideVal)
			continue
		}

		baseMap, baseIsMap := baseVal.(map[string]any)
		overrideMap, overrideIsMap := overrideVal.(map[string]any)

		if baseIsMap && overrideIsMap {
			result[k] = DeepMerge(baseMap, overrideMap)
		} else {
			result[k] = deepCopyValue(overrideVal)
		}
	}

	return result
}

// deepCopyValue returns a deep copy of an arbitrary config value, recursing
// into map[string]any and []any. Scalars are immutable and returned as-is.
// Only the map[string]any / []any shape produced by JSON and Kubernetes
// unstructured decoding is recursed into — which is exactly what
// AerospikeConfigSpec.Value holds; typed containers (e.g. []string) would
// still be aliased, but that shape never reaches DeepMerge.
func deepCopyValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		c := make(map[string]any, len(t))
		for k, val := range t {
			c[k] = deepCopyValue(val)
		}
		return c
	case []any:
		c := make([]any, len(t))
		for i, val := range t {
			c[i] = deepCopyValue(val)
		}
		return c
	default:
		return v
	}
}
