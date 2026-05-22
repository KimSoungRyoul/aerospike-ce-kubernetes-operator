package configdiff

import (
	"fmt"
	"reflect"
)

// Change represents a single configuration parameter change.
type Change struct {
	// Path is the dot-separated config path (e.g., "service.proto-fd-max").
	Path string
	// Context is the Aerospike config context (e.g., "service", "network", "namespace").
	Context string
	// Key is the parameter name within the context.
	Key string
	// OldValue is the previous value.
	OldValue any
	// NewValue is the desired value.
	NewValue any
	// Namespace is the Aerospike namespace name (for namespace-level params).
	Namespace string
}

// DiffResult contains the categorized configuration changes.
type DiffResult struct {
	// Dynamic changes that can be applied via set-config without restart.
	Dynamic []Change
	// Static changes that require a pod restart.
	Static []Change
}

// HasChanges returns true if there are any changes.
func (d *DiffResult) HasChanges() bool {
	return len(d.Dynamic) > 0 || len(d.Static) > 0
}

// HasStaticChanges returns true if any changes require a restart.
func (d *DiffResult) HasStaticChanges() bool {
	return len(d.Static) > 0
}

// Diff compares old and new Aerospike config maps and categorizes changes
// as dynamic (runtime-changeable) or static (requires restart).
func Diff(oldConfig, newConfig map[string]any) *DiffResult {
	result := &DiffResult{}
	diffSection(result, "", oldConfig, newConfig)
	return result
}

// diffSection recursively compares two config sections.
func diffSection(result *DiffResult, prefix string, oldSection, newSection map[string]any) {
	if oldSection == nil {
		oldSection = make(map[string]any)
	}
	if newSection == nil {
		newSection = make(map[string]any)
	}

	// Check for changed or added keys
	for key, newVal := range newSection {
		path := joinPath(prefix, key)
		oldVal, exists := oldSection[key]

		// Handle namespace arrays specially
		if key == "namespaces" {
			diffNamespaces(result, asSlice(oldVal), asSlice(newVal))
			continue
		}

		if !exists {
			// New key added
			classifyChange(result, path, nil, newVal, "")
			continue
		}

		// Both exist: compare
		newMap, newIsMap := newVal.(map[string]any)
		oldMap, oldIsMap := oldVal.(map[string]any)

		if newIsMap && oldIsMap {
			diffSection(result, path, oldMap, newMap)
		} else if !valuesEqual(oldVal, newVal) {
			classifyChange(result, path, oldVal, newVal, "")
		}
	}

	// Check for removed keys
	for key, oldVal := range oldSection {
		if key == "namespaces" {
			continue
		}
		if _, exists := newSection[key]; !exists {
			path := joinPath(prefix, key)
			classifyChange(result, path, oldVal, nil, "")
		}
	}
}

// diffNamespaces handles namespace-level config diff.
func diffNamespaces(result *DiffResult, oldNS, newNS []any) {
	oldByName := namespacesByName(oldNS)
	newByName := namespacesByName(newNS)

	for name, newCfg := range newByName {
		oldCfg, exists := oldByName[name]
		if !exists {
			// New namespace added — this is static (requires restart)
			result.Static = append(result.Static, Change{
				Path:     fmt.Sprintf("namespaces.%s", name),
				Context:  "namespace",
				Key:      name,
				NewValue: newCfg,
			})
			continue
		}

		// Compare namespace params
		for key, newVal := range newCfg {
			if key == "name" {
				continue
			}
			oldVal, exists := oldCfg[key]
			path := fmt.Sprintf("namespace.%s", key)

			if !exists || !valuesEqual(oldVal, newVal) {
				change := Change{
					Path:      path,
					Context:   "namespace",
					Key:       key,
					OldValue:  oldVal,
					NewValue:  newVal,
					Namespace: name,
				}
				if IsDynamic(path) {
					result.Dynamic = append(result.Dynamic, change)
				} else {
					result.Static = append(result.Static, change)
				}
			}
		}

		// Check removed keys
		for key, oldVal := range oldCfg {
			if key == "name" {
				continue
			}
			if _, exists := newCfg[key]; !exists {
				path := fmt.Sprintf("namespace.%s", key)
				result.Static = append(result.Static, Change{
					Path:      path,
					Context:   "namespace",
					Key:       key,
					OldValue:  oldVal,
					Namespace: name,
				})
			}
		}
	}

	// Check removed namespaces
	for name := range oldByName {
		if _, exists := newByName[name]; !exists {
			result.Static = append(result.Static, Change{
				Path:     fmt.Sprintf("namespaces.%s", name),
				Context:  "namespace",
				Key:      name,
				OldValue: oldByName[name],
			})
		}
	}
}

// classifyChange categorizes a change as dynamic or static.
func classifyChange(result *DiffResult, path string, oldVal, newVal any, namespace string) {
	change := Change{
		Path:      path,
		Key:       keyWithinContext(path),
		Context:   firstSegment(path),
		OldValue:  oldVal,
		NewValue:  newVal,
		Namespace: namespace,
	}

	if IsDynamic(path) {
		result.Dynamic = append(result.Dynamic, change)
	} else {
		result.Static = append(result.Static, change)
	}
}

// Helper functions

func joinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

func firstSegment(path string) string {
	for i, c := range path {
		if c == '.' {
			return path[:i]
		}
	}
	return path
}

// keyWithinContext returns the asinfo set-config parameter key for a config
// path: the path with its leading context segment (firstSegment) stripped.
//
// The asinfo "set-config" command only accepts the top-level contexts
// service / network / namespace / security / xdr. Sub-stanzas like heartbeat
// and fabric are NOT contexts — their parameters are addressed by a dotted key
// within the network context. For example "network.heartbeat.interval" must be
// applied as:
//
//	set-config:context=network;heartbeat.interval=<value>
//
// so the key is "heartbeat.interval", not just "interval". Taking only the
// last path segment produced "interval", yielding the invalid command
// "set-config:context=network;interval=<value>" which Aerospike rejects — every
// dynamic heartbeat/fabric/security-sub change then silently fell back to a
// disruptive cold restart even though it was registered as dynamic.
//
// For a two-segment path such as "service.proto-fd-max" this returns
// "proto-fd-max" (the same value the previous last-segment logic produced).
func keyWithinContext(path string) string {
	for i, c := range path {
		if c == '.' {
			return path[i+1:]
		}
	}
	return path
}

func valuesEqual(a, b any) bool {
	// Numbers must compare across Go types: a config value arrives as int
	// from a Go literal (webhook defaulter) but as float64 after a JSON
	// round-trip through the API server, so int 15000 and float64 15000
	// must be equal — otherwise the diff reports a phantom change and the
	// reconciler triggers an unnecessary restart.
	if af, aIsNum := numericValue(a); aIsNum {
		bf, bIsNum := numericValue(b)
		return bIsNum && af == bf
	}
	switch aTyped := a.(type) {
	case string:
		bTyped, ok := b.(string)
		return ok && aTyped == bTyped
	case bool:
		bTyped, ok := b.(bool)
		return ok && aTyped == bTyped
	case nil:
		return b == nil
	default:
		return reflect.DeepEqual(a, b)
	}
}

// numericValue converts any Go numeric type to float64 so values can be
// compared regardless of the concrete type they were decoded as. The second
// return is false when v is not a number.
//
// Widening to float64 is exact for Aerospike config values: every numeric
// parameter (ports, proto-fd-max, memory/data sizes, periods) is far below
// 2^53, the largest integer float64 represents exactly.
func numericValue(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

func asSlice(v any) []any {
	if v == nil {
		return nil
	}
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

func namespacesByName(namespaces []any) map[string]map[string]any {
	result := make(map[string]map[string]any, len(namespaces))
	for _, ns := range namespaces {
		if nsMap, ok := ns.(map[string]any); ok {
			if name, ok := nsMap["name"].(string); ok {
				result[name] = nsMap
			}
		}
	}
	return result
}
