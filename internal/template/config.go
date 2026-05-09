/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package template

import (
	"fmt"
	"maps"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

// applyAerospikeConfig merges template aerospikeConfig defaults into the cluster's aerospikeConfig.
// Cluster-level settings always take precedence over template defaults.
//
// Returns an error when the cluster's aerospikeConfig.service exists with a
// non-map type (which the cluster webhook rejects, but check defensively
// here so a future bypass of the webhook does not silently coerce a non-map
// service value into a fresh empty map and discard the user's data).
func applyAerospikeConfig(tmplConfig *ackov1alpha1.TemplateAerospikeConfig, cluster *ackov1alpha1.AerospikeCluster) error {
	if tmplConfig == nil {
		return nil
	}

	if cluster.Spec.AerospikeConfig == nil {
		cluster.Spec.AerospikeConfig = &ackov1alpha1.AerospikeConfigSpec{
			Value: make(map[string]any),
		}
	}
	if cluster.Spec.AerospikeConfig.Value == nil {
		cluster.Spec.AerospikeConfig.Value = make(map[string]any)
	}

	config := cluster.Spec.AerospikeConfig.Value

	// Apply service defaults (template is base, cluster overrides).
	if tmplConfig.Service != nil && len(tmplConfig.Service.Value) > 0 {
		// Defensive type check: the cluster webhook already rejects
		// aerospikeConfig.service when it isn't a map, but if that path is
		// ever bypassed (failurePolicy=Ignore on a future deployment, an
		// unrelated mutation between admission and reconcile, ...) we must
		// not silently overwrite the user's value with our merged map.
		// getOrCreateSection would return a fresh empty map in that case
		// and the deepMerge below would discard the original payload.
		if existing, exists := config["service"]; exists {
			if _, ok := existing.(map[string]any); !ok {
				return fmt.Errorf(
					"aerospikeConfig.service has type %T, expected map; refusing to overwrite with template defaults",
					existing,
				)
			}
		}
		existing := getOrCreateSection(config, "service")
		config["service"] = deepMergeMapBaseFirst(tmplConfig.Service.Value, existing)
	}

	// Apply network.heartbeat defaults.
	if tmplConfig.Network != nil && tmplConfig.Network.Heartbeat != nil {
		networkSection := getOrCreateSection(config, "network")
		heartbeatSection := getOrCreateSection(networkSection, "heartbeat")
		hb := tmplConfig.Network.Heartbeat
		if hb.Mode != "" {
			if _, exists := heartbeatSection["mode"]; !exists {
				heartbeatSection["mode"] = hb.Mode
			}
		}
		if hb.Interval > 0 {
			if _, exists := heartbeatSection["interval"]; !exists {
				heartbeatSection["interval"] = hb.Interval
			}
		}
		if hb.Timeout > 0 {
			if _, exists := heartbeatSection["timeout"]; !exists {
				heartbeatSection["timeout"] = hb.Timeout
			}
		}
		networkSection["heartbeat"] = heartbeatSection
		config["network"] = networkSection
	}

	// Apply namespace defaults to each namespace.
	if tmplConfig.NamespaceDefaults != nil && len(tmplConfig.NamespaceDefaults.Value) > 0 {
		applyNamespaceDefaults(config, tmplConfig.NamespaceDefaults.Value)
	}
	return nil
}

// applyNamespaceDefaults merges defaults into each namespace entry in aerospikeConfig.
// Each namespace's own settings take precedence over the defaults.
func applyNamespaceDefaults(config map[string]any, defaults map[string]any) {
	if len(defaults) == 0 {
		return
	}

	nsSection, ok := config["namespaces"]
	if !ok {
		return
	}

	nsList, ok := nsSection.([]any)
	if !ok {
		return
	}

	for i, ns := range nsList {
		nsMap, ok := ns.(map[string]any)
		if !ok {
			continue
		}
		// Merge: defaults is the base, nsMap overrides
		nsList[i] = deepMergeMapBaseFirst(defaults, nsMap)
	}
	config["namespaces"] = nsList
}

// getOrCreateSection returns the map at key or creates a new one.
func getOrCreateSection(m map[string]any, key string) map[string]any {
	if existing, ok := m[key]; ok {
		if existingMap, ok := existing.(map[string]any); ok {
			return existingMap
		}
	}
	newMap := make(map[string]any)
	m[key] = newMap
	return newMap
}

// deepMergeMapBaseFirst merges base and override maps recursively.
// The override map's values take precedence over base values.
// Returns a new map; neither input is modified.
func deepMergeMapBaseFirst(base, override map[string]any) map[string]any {
	result := make(map[string]any, len(base)+len(override))

	// Copy base first.
	maps.Copy(result, base)

	// Override with values from override map.
	for k, v := range override {
		if baseVal, exists := result[k]; exists {
			// If both are maps, recurse.
			baseMap, baseIsMap := baseVal.(map[string]any)
			overrideMap, overrideIsMap := v.(map[string]any)
			if baseIsMap && overrideIsMap {
				result[k] = deepMergeMapBaseFirst(baseMap, overrideMap)
				continue
			}
		}
		result[k] = v
	}

	return result
}
