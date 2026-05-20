package utils

import (
	"testing"
)

func TestDeepMerge_EmptyBase(t *testing.T) {
	base := map[string]any{}
	override := map[string]any{"key": "value"}
	result := DeepMerge(base, override)
	if result["key"] != "value" {
		t.Errorf("expected 'value', got %v", result["key"])
	}
}

func TestDeepMerge_EmptyOverride(t *testing.T) {
	base := map[string]any{"key": "value"}
	override := map[string]any{}
	result := DeepMerge(base, override)
	if result["key"] != "value" {
		t.Errorf("expected 'value', got %v", result["key"])
	}
}

func TestDeepMerge_OverrideWins(t *testing.T) {
	base := map[string]any{"key": "base-value"}
	override := map[string]any{"key": "override-value"}
	result := DeepMerge(base, override)
	if result["key"] != "override-value" {
		t.Errorf("expected 'override-value', got %v", result["key"])
	}
}

func TestDeepMerge_NestedMaps(t *testing.T) {
	base := map[string]any{
		"service": map[string]any{
			"cluster-name": "test",
			"proto-fd-max": 15000,
		},
	}
	override := map[string]any{
		"service": map[string]any{
			"proto-fd-max": 20000,
		},
	}
	result := DeepMerge(base, override)

	service, ok := result["service"].(map[string]any)
	if !ok {
		t.Fatal("expected service to be a map")
	}
	if service["cluster-name"] != "test" {
		t.Error("cluster-name should be preserved from base")
	}
	if service["proto-fd-max"] != 20000 {
		t.Errorf("proto-fd-max should be overridden to 20000, got %v", service["proto-fd-max"])
	}
}

func TestDeepMerge_DoesNotModifyInputs(t *testing.T) {
	base := map[string]any{"key": "base"}
	override := map[string]any{"key": "override"}

	DeepMerge(base, override)

	if base["key"] != "base" {
		t.Error("base map was modified")
	}
	if override["key"] != "override" {
		t.Error("override map was modified")
	}
}

func TestDeepMerge_NewKeyFromOverride(t *testing.T) {
	base := map[string]any{"a": 1}
	override := map[string]any{"b": 2}
	result := DeepMerge(base, override)
	if result["a"] != 1 || result["b"] != 2 {
		t.Errorf("expected {a:1, b:2}, got %v", result)
	}
}

func TestDeepMerge_MapOverridesScalar(t *testing.T) {
	base := map[string]any{"key": "scalar"}
	override := map[string]any{"key": map[string]any{"nested": true}}
	result := DeepMerge(base, override)
	nested, ok := result["key"].(map[string]any)
	if !ok {
		t.Fatal("expected key to be a map after override")
	}
	if nested["nested"] != true {
		t.Error("expected nested=true")
	}
}

func TestDeepMerge_ScalarOverridesMap(t *testing.T) {
	base := map[string]any{"key": map[string]any{"nested": true}}
	override := map[string]any{"key": "scalar"}
	result := DeepMerge(base, override)
	if result["key"] != "scalar" {
		t.Errorf("expected scalar, got %v", result["key"])
	}
}

func TestDeepMerge_ResultDoesNotAliasNestedBaseMap(t *testing.T) {
	base := map[string]any{
		"service": map[string]any{"proto-fd-max": 15000},
	}
	override := map[string]any{}
	result := DeepMerge(base, override)

	// Mutating a nested map in the result must not reach back into base.
	result["service"].(map[string]any)["proto-fd-max"] = 999
	if base["service"].(map[string]any)["proto-fd-max"] != 15000 {
		t.Error("mutating result aliased back into base map")
	}
}

func TestDeepMerge_ResultDoesNotAliasNestedOverrideMap(t *testing.T) {
	base := map[string]any{}
	override := map[string]any{
		"network": map[string]any{"port": 3000},
	}
	result := DeepMerge(base, override)

	// A key present only in override must also be deep-copied.
	result["network"].(map[string]any)["port"] = 9999
	if override["network"].(map[string]any)["port"] != 3000 {
		t.Error("mutating result aliased back into override map")
	}
}

func TestDeepMerge_ResultDoesNotAliasSlices(t *testing.T) {
	base := map[string]any{
		"namespaces": []any{
			map[string]any{"name": "test"},
		},
	}
	result := DeepMerge(base, map[string]any{})

	result["namespaces"].([]any)[0].(map[string]any)["name"] = "mutated"
	if base["namespaces"].([]any)[0].(map[string]any)["name"] != "test" {
		t.Error("mutating result aliased back into base slice element")
	}
}

// --- IntFromAny tests ---

func TestIntFromAny_Int(t *testing.T) {
	if got := IntFromAny(42, 0); got != 42 {
		t.Errorf("IntFromAny(42) = %d, want 42", got)
	}
}

func TestIntFromAny_Int64(t *testing.T) {
	if got := IntFromAny(int64(3000), 0); got != 3000 {
		t.Errorf("IntFromAny(int64(3000)) = %d, want 3000", got)
	}
}

func TestIntFromAny_Float64(t *testing.T) {
	if got := IntFromAny(float64(3002), 0); got != 3002 {
		t.Errorf("IntFromAny(float64(3002)) = %d, want 3002", got)
	}
}

func TestIntFromAny_String_ReturnsFallback(t *testing.T) {
	if got := IntFromAny("not-a-number", 99); got != 99 {
		t.Errorf("IntFromAny(string) = %d, want 99", got)
	}
}

func TestIntFromAny_Nil_ReturnsFallback(t *testing.T) {
	if got := IntFromAny(nil, 3000); got != 3000 {
		t.Errorf("IntFromAny(nil) = %d, want 3000", got)
	}
}

// --- ShortSHA256 tests ---

func TestShortSHA256_Deterministic(t *testing.T) {
	h1 := ShortSHA256(map[string]any{"key": "value"})
	h2 := ShortSHA256(map[string]any{"key": "value"})
	if h1 != h2 {
		t.Errorf("ShortSHA256 should be deterministic: %q != %q", h1, h2)
	}
}

func TestShortSHA256_DifferentValues(t *testing.T) {
	h1 := ShortSHA256(map[string]any{"key": "value1"})
	h2 := ShortSHA256(map[string]any{"key": "value2"})
	if h1 == h2 {
		t.Error("ShortSHA256 should produce different hashes for different values")
	}
}

func TestShortSHA256_Format(t *testing.T) {
	h := ShortSHA256(map[string]any{"key": "value"})
	if len(h) != 16 {
		t.Errorf("expected hex length 16 (8 bytes), got %d", len(h))
	}
}

func TestShortSHA256_NilValue(t *testing.T) {
	h := ShortSHA256(nil)
	if h == "" {
		t.Error("nil should still produce a hash (json null)")
	}
}
