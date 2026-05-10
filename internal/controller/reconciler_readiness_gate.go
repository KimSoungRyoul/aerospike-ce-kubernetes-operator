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

package controller

import (
	"context"
	"time"

	aero "github.com/aerospike/aerospike-client-go/v8"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/podutil"
)

// syncAllPodsReadinessGates syncs the "acko.io/aerospike-ready" pod condition
// for every running pod in the cluster when spec.podSpec.readinessGateEnabled=true.
// When the feature is disabled, it is a no-op. Gate sync errors are non-fatal:
// if the patch fails, the gate remains False, which safely holds the rolling restart.
func (r *AerospikeClusterReconciler) syncAllPodsReadinessGates(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) error {
	if !isReadinessGateEnabled(cluster) {
		return nil
	}

	log := logf.FromContext(ctx)

	podList, err := r.listClusterPods(ctx, cluster)
	if err != nil {
		return err
	}

	// Create a single cluster-level Aerospike client for all pod checks.
	aeroClient, err := r.getAerospikeClient(ctx, cluster)
	if err != nil {
		// Aerospike unreachable — leave gate as-is. Rolling restart will
		// be held by anyPodGateUnsatisfied() which is the safe behavior.
		log.V(1).Info("Could not connect to Aerospike for readiness gate sync; skipping", "err", err)
		return nil
	}
	defer closeAerospikeClient(aeroClient)

	// Cluster-wide migration check, called once before the loop rather than
	// repeating it for every pod. Use isMigratingOnAnyNode (per-node
	// migrate_partitions_remaining) so the readiness gate, scaledown, and
	// rolling-restart paths all see the same migration signal. The previous
	// single-node cluster-stable check could clear the gate while a remote
	// node still had outstanding migrations, producing a false-negative that
	// allowed a rolling restart to proceed during data movement.
	migrating, migratingErr := isMigratingOnAnyNode(aeroClient)

	for i := range podList.Items {
		pod := &podList.Items[i]
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if pod.DeletionTimestamp != nil {
			continue
		}
		if err := r.syncPodReadinessGate(ctx, cluster, pod, aeroClient, migrating, migratingErr); err != nil {
			log.Error(err, "Failed to sync readiness gate", "pod", pod.Name)
			// Continue to next pod; gate patch errors are per-pod non-fatal.
		}
	}
	return nil
}

// syncPodReadinessGate checks Aerospike cluster health for a single pod and
// patches pod.Status.Conditions to reflect the "acko.io/aerospike-ready" gate.
// The migrating/migratingErr parameters are the pre-computed cluster-wide
// migration state, hoisted out of the per-pod loop by the caller.
func (r *AerospikeClusterReconciler) syncPodReadinessGate(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	pod *corev1.Pod,
	aeroClient *aero.Client,
	migrating bool,
	migratingErr error,
) error {
	log := logf.FromContext(ctx)

	// Only patch pods that have the readiness gate injected in their spec.
	// Pods created before the feature was enabled won't have the gate.
	if !podHasReadinessGate(pod) {
		return nil
	}

	// Determine desired gate state.
	satisfied := false

	// 1. Check that this pod's Aerospike node has joined the mesh.
	node := findNodeForPod(aeroClient, pod)
	if node != nil {
		// 2. Check that the cluster has no pending migrations.
		if migratingErr != nil {
			log.V(1).Info("migration check failed; treating as migrating", "pod", pod.Name, "err", migratingErr)
		} else {
			satisfied = !migrating
		}
	}

	// Avoid unnecessary patch when desired state matches existing condition.
	existing, exists := findPodReadinessCondition(pod)
	if exists && existing == satisfied {
		return nil
	}

	// Emit event on gate transition from False -> True.
	if satisfied {
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventReadinessGateSatisfied,
			"Readiness gate satisfied for pod %s: Aerospike has joined the mesh and all migrations are complete", pod.Name)
	}

	return r.patchPodReadinessCondition(ctx, pod, satisfied)
}

// patchPodReadinessCondition patches pod.Status.Conditions to set or update
// the "acko.io/aerospike-ready" condition via the pods/status subresource.
func (r *AerospikeClusterReconciler) patchPodReadinessCondition(
	ctx context.Context,
	pod *corev1.Pod,
	satisfied bool,
) error {
	podCopy := pod.DeepCopy()

	condStatus := corev1.ConditionFalse
	reason := "MigrationsInProgress"
	message := "Aerospike has not joined the cluster mesh or data migrations are still in progress"
	if satisfied {
		condStatus = corev1.ConditionTrue
		reason = "AerospikeReady"
		message = "Aerospike has joined the cluster mesh and all data migrations are complete"
	}

	upsertPodCondition(podCopy, corev1.PodCondition{
		Type:               podutil.AerospikeReadinessGateConditionType,
		Status:             condStatus,
		LastTransitionTime: metav1.NewTime(time.Now()),
		Reason:             reason,
		Message:            message,
	})

	return r.Status().Patch(ctx, podCopy, client.MergeFrom(pod))
}

// findPodReadinessCondition returns the current "acko.io/aerospike-ready" condition
// status and whether it exists.
func findPodReadinessCondition(pod *corev1.Pod) (satisfied bool, exists bool) {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == podutil.AerospikeReadinessGateConditionType {
			return cond.Status == corev1.ConditionTrue, true
		}
	}
	return false, false
}

// upsertPodCondition updates an existing pod condition in-place or appends a new one.
// LastTransitionTime is only updated when the Status changes.
func upsertPodCondition(pod *corev1.Pod, newCond corev1.PodCondition) {
	for i, cond := range pod.Status.Conditions {
		if cond.Type == newCond.Type {
			if cond.Status == newCond.Status {
				// Status unchanged — preserve the original LastTransitionTime.
				newCond.LastTransitionTime = cond.LastTransitionTime
			}
			pod.Status.Conditions[i] = newCond
			return
		}
	}
	pod.Status.Conditions = append(pod.Status.Conditions, newCond)
}

// isPodReadinessGateSatisfied returns true if the readiness gate feature is
// disabled, or if the pod's "acko.io/aerospike-ready" condition is True.
// Pods that predate the feature (gate not in spec) are treated as satisfied.
func isPodReadinessGateSatisfied(cluster *ackov1alpha1.AerospikeCluster, pod *corev1.Pod) bool {
	if !isReadinessGateEnabled(cluster) {
		return true
	}
	if !podHasReadinessGate(pod) {
		// Pod was created before the feature was enabled; treat as satisfied
		// so it does not block rolling restart indefinitely.
		return true
	}
	satisfied, _ := findPodReadinessCondition(pod)
	return satisfied
}

// isReadinessGateEnabled returns true when spec.podSpec.readinessGateEnabled=true.
func isReadinessGateEnabled(cluster *ackov1alpha1.AerospikeCluster) bool {
	return cluster.Spec.PodSpec != nil &&
		cluster.Spec.PodSpec.ReadinessGateEnabled != nil &&
		*cluster.Spec.PodSpec.ReadinessGateEnabled
}

// podHasReadinessGate returns true if the pod's spec contains the
// "acko.io/aerospike-ready" readiness gate.
func podHasReadinessGate(pod *corev1.Pod) bool {
	for _, rg := range pod.Spec.ReadinessGates {
		if rg.ConditionType == podutil.AerospikeReadinessGateConditionType {
			return true
		}
	}
	return false
}

// anyPodGateUnsatisfied returns true if any Running, non-Terminating pod in
// rackPods has the readiness gate in its spec but the condition is not yet True.
// Returns the name of the first such pod for logging.
func anyPodGateUnsatisfied(
	cluster *ackov1alpha1.AerospikeCluster,
	rackPods []corev1.Pod,
) (blocked bool, podName string) {
	for i := range rackPods {
		pod := &rackPods[i]
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if pod.DeletionTimestamp != nil {
			continue
		}
		if !isPodReadinessGateSatisfied(cluster, pod) {
			return true, pod.Name
		}
	}
	return false, ""
}
