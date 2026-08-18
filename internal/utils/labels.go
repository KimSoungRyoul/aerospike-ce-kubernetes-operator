package utils

import "fmt"

const (
	AppLabel       = "app.kubernetes.io/name"
	InstanceLabel  = "app.kubernetes.io/instance"
	ComponentLabel = "app.kubernetes.io/component"
	ManagedByLabel = "app.kubernetes.io/managed-by"
	RackLabel      = "acko.io/rack"

	ConfigHashAnnotation  = "acko.io/config-hash"
	PodSpecHashAnnotation = "acko.io/podspec-hash"

	// StorageHashAnnotation carries the hash of the rack's effective storage
	// spec, SEPARATE from PodSpecHashAnnotation on purpose.
	//
	// Storage was briefly folded into the pod-spec hash (#340/#359). That changed
	// the hash of every cluster that has any spec.storage, with no user edit, so
	// upgrading the operator queued every pod of every cluster for a cold restart
	// — each with a full data migration, and during exactly the window #341 exists
	// to keep pods from being deleted in.
	//
	// Keeping storage in its own annotation leaves the pod-spec hash byte-identical
	// across the upgrade. A pod that predates this annotation simply does not carry
	// it, and an absent value is treated as matching rather than as a mismatch.
	StorageHashAnnotation = "acko.io/storage-hash"
	StorageFinalizer      = "acko.io/storage-finalizer"

	appName     = "aerospike-cluster"
	managerName = "aerospike-ce-kubernetes-operator"
)

// LabelsForCluster returns the common labels for resources belonging to a cluster.
func LabelsForCluster(clusterName string) map[string]string {
	return map[string]string{
		AppLabel:       appName,
		InstanceLabel:  clusterName,
		ComponentLabel: "database",
		ManagedByLabel: managerName,
	}
}

// SelectorLabelsForCluster returns the minimal label set used for label selectors.
func SelectorLabelsForCluster(clusterName string) map[string]string {
	return map[string]string{
		AppLabel:      appName,
		InstanceLabel: clusterName,
	}
}

// LabelsForRack returns labels for a specific rack, including the rack ID.
func LabelsForRack(clusterName string, rackID int) map[string]string {
	labels := LabelsForCluster(clusterName)
	labels[RackLabel] = fmt.Sprintf("%d", rackID)
	return labels
}
