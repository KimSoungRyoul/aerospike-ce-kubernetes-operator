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
	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

// applyImage applies the template image default to the cluster.
// Only applied when the cluster's spec.image is empty (not explicitly set).
func applyImage(tmplImage string, cluster *ackov1alpha1.AerospikeCluster) {
	if tmplImage == "" {
		return
	}
	if cluster.Spec.Image == "" {
		cluster.Spec.Image = tmplImage
	}
}

// applySize applies the template size default to the cluster.
// Only applied when the cluster's spec.size is 0 (not explicitly set).
// Valid cluster sizes are 1–8; zero is the zero value meaning "unset".
func applySize(tmplSize *int32, cluster *ackov1alpha1.AerospikeCluster) {
	if tmplSize == nil {
		return
	}
	if cluster.Spec.Size == 0 {
		cluster.Spec.Size = *tmplSize
	}
}

// applyMonitoring applies the template monitoring defaults to the cluster.
// Only applied when the cluster does not already have monitoring configured.
func applyMonitoring(tmplMonitoring *ackov1alpha1.AerospikeMonitoringSpec, cluster *ackov1alpha1.AerospikeCluster) {
	if tmplMonitoring == nil {
		return
	}
	if cluster.Spec.Monitoring == nil {
		cluster.Spec.Monitoring = tmplMonitoring.DeepCopy()
	}
}

// applyNetworkPolicy applies the template network policy defaults to the cluster.
// Only applied when the cluster does not already have a network policy configured.
func applyNetworkPolicy(tmplPolicy *ackov1alpha1.AerospikeNetworkPolicy, cluster *ackov1alpha1.AerospikeCluster) {
	if tmplPolicy == nil {
		return
	}
	if cluster.Spec.AerospikeNetworkPolicy == nil {
		cluster.Spec.AerospikeNetworkPolicy = tmplPolicy.DeepCopy()
	}
}
