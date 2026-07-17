package controller

import (
	"context"

	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

// isMigrationInProgress connects to the Aerospike cluster and checks whether
// data migration is currently running on any node.
//
// Returns (true, nil) when migration is active, (false, nil) when all nodes
// are stable, and (false, err) when the Aerospike cluster cannot be reached.
// The caller decides how to handle connection errors -- typically logging
// the error and proceeding so that an unreachable cluster does not permanently
// block scale-down.
func (r *AerospikeClusterReconciler) isMigrationInProgress(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) (bool, error) {
	log := logf.FromContext(ctx)

	aeroClient, err := r.getAerospikeClient(ctx, cluster)
	if err != nil {
		return false, err
	}
	defer closeAerospikeClient(aeroClient)

	migrating, err := isMigratingOnAnyNode(aeroClient)
	if err != nil {
		log.V(1).Info("Migration check failed", "error", err)
		return false, err
	}

	return migrating, nil
}
