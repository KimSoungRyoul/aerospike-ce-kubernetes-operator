package controller

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	aero "github.com/aerospike/aerospike-client-go/v8"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/configdiff"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/metrics"
)

const (
	// perPodDynamicConfigTimeout is the maximum duration for validate/apply
	// operations on a single pod. Independent of the reconciliation-level timeout.
	// 30s × 8 pods (CE max) = 240s < 300s reconcile timeout.
	perPodDynamicConfigTimeout = 30 * time.Second
)

// appliedChange tracks a successfully applied dynamic config change along with
// the command needed to roll it back.
type appliedChange struct {
	change   configdiff.Change
	rollback string // the set-config command to revert this change; empty if rollback not possible
}

// RollbackResult tracks the outcome of a rollback operation across one or more pods.
type RollbackResult struct {
	SuccessCount int
	FailedCount  int
	FailedPods   []string
}

// HasFailures returns true if any rollback operations failed.
func (r RollbackResult) HasFailures() bool {
	return r.FailedCount > 0
}

// validateDynamicConfigOnPod performs the "prepare" phase of 2PC for a single pod.
// It validates that all changes can be built into valid set-config commands and
// that the Aerospike node is responsive. Returns an error if validation fails.
func (r *AerospikeClusterReconciler) validateDynamicConfigOnPod(
	node *aero.Node,
	changes []configdiff.Change,
) error {
	// Validate command syntax
	if err := validateDynamicChanges(changes); err != nil {
		return fmt.Errorf("pre-flight validation failed: %w", err)
	}

	// Probe node responsiveness with a lightweight info command
	if _, err := asinfoCommandOnNode(node, "node"); err != nil {
		return fmt.Errorf("node responsiveness check failed: %w", err)
	}

	return nil
}

// applyDynamicConfigOnPod applies dynamic config changes to a single Aerospike node.
// Returns the list of successfully applied changes and whether all changes succeeded.
func (r *AerospikeClusterReconciler) applyDynamicConfigOnPod(
	ctx context.Context,
	node *aero.Node,
	changes []configdiff.Change,
) ([]appliedChange, bool) {
	log := logf.FromContext(ctx)
	var applied []appliedChange

	for i, change := range changes {
		// Check context deadline before each command
		if ctx.Err() != nil {
			log.Error(ctx.Err(), "Per-pod timeout exceeded during dynamic config apply",
				"appliedSoFar", len(applied), "remaining", len(changes)-i)
			return applied, false
		}

		cmd, err := buildSetConfigCommand(change)
		if err != nil {
			log.Error(err, "Invalid dynamic config change", "change", change)
			logAppliedChanges(log, applied, i, len(changes))
			return applied, false
		}
		log.Info("Applying dynamic config", "command", cmd, "index", i, "total", len(changes))

		result, err := asinfoCommandOnNode(node, cmd)
		if err != nil {
			log.Error(err, "Dynamic config command failed", "command", cmd)
			logAppliedChanges(log, applied, i, len(changes))
			return applied, false
		}
		if result != "ok" {
			log.Info("Dynamic config command returned non-ok", "command", cmd, "result", result)
			logAppliedChanges(log, applied, i, len(changes))
			return applied, false
		}

		// Build rollback command using the old value.
		rollbackCmd := buildRollbackCommand(log, change)
		applied = append(applied, appliedChange{change: change, rollback: rollbackCmd})
	}

	return applied, true
}

// tryDynamicConfigUpdateBatch implements Two-Phase Commit for dynamic config
// updates across multiple pods.
// Phase 1 (Validate): Validate on all pods. If ANY pod fails, abort entirely.
// Phase 2 (Apply): Apply on all pods. If any pod fails, rollback ALL pods.
// Returns (allSucceeded, perPodResults, rollbackResult).
func (r *AerospikeClusterReconciler) tryDynamicConfigUpdateBatch(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	pods []*corev1.Pod,
	oldConfig, newConfig map[string]any,
	aeroClient *aero.Client,
) (bool, []podDynamicUpdate, *RollbackResult) {
	log := logf.FromContext(ctx).WithValues("cluster", cluster.Name)

	// Check if dynamic config update is enabled
	if cluster.Spec.EnableDynamicConfigUpdate == nil || !*cluster.Spec.EnableDynamicConfigUpdate {
		return false, nil, nil
	}

	// Diff the configs
	diff := configdiff.Diff(oldConfig, newConfig)
	if !diff.HasChanges() {
		return true, nil, nil
	}

	if diff.HasStaticChanges() {
		log.Info("Config has static changes, dynamic update not sufficient",
			"staticChanges", len(diff.Static))
		return false, nil, nil
	}

	// Resolve nodes for all pods
	type podNode struct {
		pod  *corev1.Pod
		node *aero.Node
	}
	var targets []podNode
	for _, pod := range pods {
		node := findNodeForPod(aeroClient, pod)
		if node == nil {
			log.Info("Could not find Aerospike node for pod, aborting batch dynamic update", "pod", pod.Name)
			return false, nil, nil
		}
		targets = append(targets, podNode{pod: pod, node: node})
	}

	// === Phase 1: Validate on all pods ===
	log.Info("2PC Phase 1: Validating dynamic config on all pods", "podCount", len(targets), "changes", len(diff.Dynamic))
	for _, t := range targets {
		// Check remaining reconcile context before each pod
		if ctx.Err() != nil {
			log.Error(ctx.Err(), "Reconcile context expired during 2PC validation phase")
			return false, nil, nil
		}

		err := r.validateDynamicConfigOnPod(t.node, diff.Dynamic)

		if err != nil {
			log.Info("2PC Phase 1 failed: validation failed on pod, aborting batch",
				"pod", t.pod.Name, "error", err)
			return false, nil, nil
		}
	}
	log.Info("2PC Phase 1 passed: all pods validated successfully")

	// === Phase 2: Apply on all pods ===
	log.Info("2PC Phase 2: Applying dynamic config on all pods")
	var successfulUpdates []podDynamicUpdate
	desiredHash := configHash(&ackov1alpha1.AerospikeConfigSpec{Value: newConfig})

	for _, t := range targets {
		// Check remaining reconcile context before each pod
		if ctx.Err() != nil {
			log.Error(ctx.Err(), "Reconcile context expired during 2PC apply phase",
				"appliedPods", len(successfulUpdates))
			// Rollback all previously applied pods
			rbResult := r.rollbackDynamicChangesBatch(log, cluster, successfulUpdates)
			return false, nil, rbResult
		}

		podCtx, podCancel := context.WithTimeout(ctx, perPodDynamicConfigTimeout)
		podCtx = logf.IntoContext(podCtx, log.WithValues("pod", t.pod.Name))
		applied, ok := r.applyDynamicConfigOnPod(podCtx, t.node, diff.Dynamic)
		podCancel()

		if !ok {
			log.Info("2PC Phase 2 failed: apply failed on pod, rolling back all pods",
				"failedPod", t.pod.Name, "appliedPods", len(successfulUpdates))

			// Rollback the failed pod's partial changes
			r.rollbackDynamicChanges(log.WithValues("pod", t.pod.Name), t.node, applied)

			// Rollback all previously successful pods
			rbResult := r.rollbackDynamicChangesBatch(log, cluster, successfulUpdates)
			return false, nil, rbResult
		}

		// Update pod config hash and status
		if desiredHash != "" {
			if err := r.updatePodConfigHash(ctx, t.pod, desiredHash); err != nil {
				log.Error(err, "Failed to update pod config hash after dynamic update", "pod", t.pod.Name)
			}
		}

		successfulUpdates = append(successfulUpdates, podDynamicUpdate{
			podName: t.pod.Name,
			node:    t.node,
			applied: applied,
		})

		metrics.DynamicConfigUpdatesTotal.WithLabelValues(cluster.Namespace, cluster.Name).Inc()
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventDynamicConfigApplied,
			"Dynamic config applied to pod %s (%d changes)", t.pod.Name, len(diff.Dynamic))
		r.updateDynamicConfigStatus(ctx, cluster, t.pod.Name, "Applied")
		r.updateDynamicConfigChanges(ctx, cluster, t.pod.Name, applied)
	}

	log.Info("2PC completed successfully: all pods updated", "podCount", len(successfulUpdates))
	return true, successfulUpdates, nil
}

// rollbackDynamicChangesBatch rolls back dynamic config changes on all pods that
// were successfully updated. Returns a RollbackResult summarizing the outcome.
func (r *AerospikeClusterReconciler) rollbackDynamicChangesBatch(
	log logr.Logger,
	cluster *ackov1alpha1.AerospikeCluster,
	updates []podDynamicUpdate,
) *RollbackResult {
	result := &RollbackResult{}

	// Rollback in reverse order (LIFO) for proper 2PC semantics:
	// the last pod to receive changes is rolled back first.
	for i := len(updates) - 1; i >= 0; i-- {
		du := updates[i]
		podLog := log.WithValues("pod", du.podName)
		podResult := r.rollbackDynamicChangesWithResult(podLog, du.node, du.applied)

		if podResult.HasFailures() {
			result.FailedCount++
			result.FailedPods = append(result.FailedPods, du.podName)
			r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventDynamicConfigRollback,
				"Rollback partially failed on pod %s: %d/%d changes rolled back",
				du.podName, podResult.SuccessCount, podResult.SuccessCount+podResult.FailedCount)
		} else {
			result.SuccessCount++
			r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventDynamicConfigRollback,
				"Rollback succeeded on pod %s: %d changes reverted", du.podName, podResult.SuccessCount)
		}
	}

	return result
}

// rollbackDynamicChangesWithResult attempts to revert previously applied dynamic
// config changes and returns a RollbackResult with per-change tracking.
func (r *AerospikeClusterReconciler) rollbackDynamicChangesWithResult(
	log logr.Logger,
	node *aero.Node,
	applied []appliedChange,
) RollbackResult {
	result := RollbackResult{}

	if len(applied) == 0 {
		return result
	}

	log.Info("Attempting rollback of applied dynamic config changes", "count", len(applied))

	for i := len(applied) - 1; i >= 0; i-- {
		ac := applied[i]
		if ac.rollback == "" {
			log.Info("No rollback command available for change, skipping",
				"change", ac.change.Path, "appliedValue", ac.change.NewValue)
			result.FailedCount++
			continue
		}

		log.Info("Rolling back dynamic config change", "command", ac.rollback, "change", ac.change.Path)
		res, err := asinfoCommandOnNode(node, ac.rollback)
		if err != nil {
			log.Error(err, "Rollback command failed", "command", ac.rollback, "change", ac.change.Path)
			result.FailedCount++
			continue
		}
		if res != "ok" {
			log.Info("Rollback command returned non-ok", "command", ac.rollback, "result", res, "change", ac.change.Path)
			result.FailedCount++
			continue
		}
		log.Info("Successfully rolled back dynamic config change", "change", ac.change.Path)
		result.SuccessCount++
	}

	return result
}

// rollbackDynamicChanges attempts to revert previously applied dynamic config changes.
// This is best-effort: if rollback fails, it is logged but does not cause an error.
// The caller will fall back to a cold restart which applies the correct config.
func (r *AerospikeClusterReconciler) rollbackDynamicChanges(
	log logr.Logger,
	node *aero.Node,
	applied []appliedChange,
) {
	result := r.rollbackDynamicChangesWithResult(log, node, applied)

	if result.HasFailures() {
		log.Info("Some rollback commands failed, cold restart will apply correct config",
			"failedRollbacks", result.FailedCount, "totalApplied", len(applied))
	} else if len(applied) > 0 {
		log.Info("All applied dynamic config changes rolled back successfully")
	}
}

// buildRollbackCommand constructs a set-config command to revert a change to its old value.
// Returns empty string if rollback is not possible (e.g., new key with no old value).
func buildRollbackCommand(log logr.Logger, change configdiff.Change) string {
	if change.OldValue == nil {
		log.V(1).Info("No old value for change, rollback not possible", "change", change.Path)
		return ""
	}
	rollbackChange := configdiff.Change{
		Path:      change.Path,
		Context:   change.Context,
		Key:       change.Key,
		NewValue:  change.OldValue,
		Namespace: change.Namespace,
	}
	cmd, err := buildSetConfigCommand(rollbackChange)
	if err != nil {
		log.V(1).Info("Cannot build rollback command for applied change",
			"change", change.Path, "oldValue", change.OldValue, "error", err)
		return ""
	}
	return cmd
}

// tomlBareKeyRe matches a safe asinfo bare key: alphanumerics, underscores,
// hyphens and dots (dots are used for nested keys like heartbeat.interval).
// Mirrors the webhook's tomlBareKeyRe validation, extended to allow the dotted
// key paths that dynamic config changes carry.
var tomlBareKeyRe = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// hasUnsafeValueChars reports whether s contains characters that could break or
// inject into the asinfo set-config wire format. The asinfo protocol is
// sensitive to ';' and ':' (directive/field separators), '=' (key/value
// separator), ASCII control characters such as '\n', '\r', '\t' (which can
// terminate or splice a directive) and leading/trailing whitespace.
func hasUnsafeValueChars(s string) (bool, string) {
	if strings.ContainsAny(s, ";:=") {
		return true, "must not contain ';', ':' or '='"
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f || unicode.IsControl(r) {
			return true, "must not contain control characters (e.g. newline, carriage return, tab)"
		}
	}
	if strings.TrimSpace(s) != s {
		return true, "must not contain leading or trailing whitespace"
	}
	return false, ""
}

// buildSetConfigCommand builds the asinfo set-config command for a change.
// Returns an error if any field contains characters that could break or inject
// into the asinfo protocol. Keys must be safe bare keys (tomlBareKeyRe); the
// context, namespace id and value must be free of asinfo delimiters (';', ':',
// '='), ASCII control characters and surrounding whitespace.
func buildSetConfigCommand(change configdiff.Change) (string, error) {
	valueStr := fmt.Sprintf("%v", change.NewValue)

	// Key must be a safe bare key (no delimiters, control chars or whitespace).
	if !tomlBareKeyRe.MatchString(change.Key) {
		return "", fmt.Errorf("invalid character in key %q: must match %s", change.Key, tomlBareKeyRe.String())
	}

	for _, field := range []struct{ name, val string }{
		{"context", change.Context},
		{"namespace", change.Namespace},
		{"value", valueStr},
	} {
		if unsafe, reason := hasUnsafeValueChars(field.val); unsafe {
			return "", fmt.Errorf("invalid character in %s %q: %s", field.name, field.val, reason)
		}
	}

	if change.Namespace != "" {
		// Namespace-scoped parameter
		return fmt.Sprintf("set-config:context=namespace;id=%s;%s=%v",
			change.Namespace, change.Key, change.NewValue), nil
	}

	return fmt.Sprintf("set-config:context=%s;%s=%v",
		change.Context, change.Key, change.NewValue), nil
}

// validateDynamicChanges performs pre-flight validation on all dynamic config
// changes to catch obvious errors before applying any of them. This prevents
// partial config state where some changes succeed and others fail.
func validateDynamicChanges(changes []configdiff.Change) error {
	var errs []string
	for _, change := range changes {
		if _, err := buildSetConfigCommand(change); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("pre-flight validation failed for %d change(s): %s",
			len(errs), strings.Join(errs, "; "))
	}
	return nil
}

// findNodeForPod finds the Aerospike node that corresponds to a given pod by
// matching the pod IP. Returns nil if no match is found (no single-node fallback
// to avoid applying config to the wrong node).
func findNodeForPod(aeroClient *aero.Client, pod *corev1.Pod) *aero.Node {
	podIP := pod.Status.PodIP
	if podIP == "" {
		return nil
	}

	for _, node := range aeroClient.GetNodes() {
		host := node.GetHost()
		if host != nil && host.Name == podIP {
			return node
		}
	}

	return nil
}

// updateDynamicConfigStatus updates the DynamicConfigStatus field in the pod's
// status within the cluster CR. Uses Patch (MergeFrom) for atomic updates to
// avoid race conditions with concurrent reconcile loops.
// Failures are non-fatal: they are logged and reported as warning Events since
// the caller cannot meaningfully retry.
func (r *AerospikeClusterReconciler) updateDynamicConfigStatus(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	podName, status string,
) {
	log := logf.FromContext(ctx)

	// Re-fetch the cluster to get the latest status
	latest := &ackov1alpha1.AerospikeCluster{}
	if err := r.Get(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, latest); err != nil {
		log.Error(err, "Failed to re-fetch cluster for dynamic config status update", "pod", podName)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventDynamicConfigStatusFailed,
			"Failed to update dynamic config status for pod %s: %v", podName, err)
		return
	}

	if latest.Status.Pods == nil {
		latest.Status.Pods = make(map[string]ackov1alpha1.AerospikePodStatus)
	}

	base := latest.DeepCopy()
	podStatus := latest.Status.Pods[podName]
	podStatus.DynamicConfigStatus = status
	latest.Status.Pods[podName] = podStatus
	if err := r.Status().Patch(ctx, latest, client.MergeFrom(base)); err != nil {
		log.Error(err, "Failed to patch dynamic config status", "pod", podName)
		r.Recorder.Eventf(cluster, corev1.EventTypeWarning, EventDynamicConfigStatusFailed,
			"Failed to update dynamic config status for pod %s: %v", podName, err)
	}
}

// updateDynamicConfigChanges records per-change details in the pod's status.
func (r *AerospikeClusterReconciler) updateDynamicConfigChanges(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	podName string,
	applied []appliedChange,
) {
	log := logf.FromContext(ctx)

	latest := &ackov1alpha1.AerospikeCluster{}
	if err := r.Get(ctx, types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, latest); err != nil {
		log.V(1).Info("Failed to re-fetch cluster for dynamic config changes update", "pod", podName, "error", err)
		return
	}

	if latest.Status.Pods == nil {
		latest.Status.Pods = make(map[string]ackov1alpha1.AerospikePodStatus)
	}

	base := latest.DeepCopy()
	podStatus := latest.Status.Pods[podName]

	changes := make([]ackov1alpha1.DynamicConfigChangeStatus, 0, len(applied))
	for _, ac := range applied {
		oldVal := ""
		if ac.change.OldValue != nil {
			oldVal = fmt.Sprintf("%v", ac.change.OldValue)
		}
		changes = append(changes, ackov1alpha1.DynamicConfigChangeStatus{
			Path:     ac.change.Path,
			OldValue: oldVal,
			NewValue: fmt.Sprintf("%v", ac.change.NewValue),
			Result:   "Applied",
		})
	}
	podStatus.DynamicConfigChanges = changes
	latest.Status.Pods[podName] = podStatus

	if err := r.Status().Patch(ctx, latest, client.MergeFrom(base)); err != nil {
		log.V(1).Info("Failed to patch dynamic config changes", "pod", podName, "error", err)
	}
}

// logAppliedChanges logs which changes were successfully applied before a failure,
// so operators can investigate partial config state if needed.
func logAppliedChanges(log logr.Logger, applied []appliedChange, failedIdx, total int) {
	if len(applied) == 0 {
		return
	}
	paths := make([]string, 0, len(applied))
	for _, ac := range applied {
		paths = append(paths, fmt.Sprintf("%s=%v", ac.change.Path, ac.change.NewValue))
	}
	log.Info("Dynamic config partially applied before failure",
		"appliedCount", len(applied), "failedAtIndex", failedIdx, "totalChanges", total,
		"appliedChanges", strings.Join(paths, ", "))
}
