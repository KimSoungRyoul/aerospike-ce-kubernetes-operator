//go:build e2e

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

package e2e

import (
	"os"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

// Shared constants used across all e2e test files.
const (
	aerospikeNS      = "aerospike"
	defaultTimeout   = 3 * time.Minute
	multiNodeTimeout = 5 * time.Minute
)

// newTestCluster creates a minimal AerospikeCluster for e2e testing.
// Use the variadic mutators to customize per-test (e.g., set batch size, paused, etc.).
func newTestCluster(name, ns string, size int32,
	mutators ...func(*ackov1alpha1.AerospikeCluster),
) *ackov1alpha1.AerospikeCluster {
	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size:  size,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeConfig: &ackov1alpha1.AerospikeConfigSpec{
				Value: map[string]any{
					"service": map[string]any{
						"cluster-name": name,
						"proto-fd-max": float64(15000),
					},
					"network": map[string]any{
						"service":   map[string]any{"address": "any", "port": float64(3000)},
						"heartbeat": map[string]any{"mode": "mesh", "port": float64(3002)},
						"fabric":    map[string]any{"address": "any", "port": float64(3001)},
					},
					"namespaces": []any{
						map[string]any{
							"name":               "test",
							"replication-factor": float64(1),
							"storage-engine": map[string]any{
								"type":      "memory",
								"data-size": float64(1073741824),
							},
						},
					},
				},
			},
		},
	}
	for _, fn := range mutators {
		fn(cluster)
	}
	return cluster
}

// loadClusterFromFile reads a sample YAML file into a typed AerospikeCluster.
// This ensures the sample files are parseable while still using the typed client for creation.
func loadClusterFromFile(path string) (*ackov1alpha1.AerospikeCluster, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cluster := &ackov1alpha1.AerospikeCluster{}
	if err := yaml.Unmarshal(data, cluster); err != nil {
		return nil, err
	}
	return cluster, nil
}
