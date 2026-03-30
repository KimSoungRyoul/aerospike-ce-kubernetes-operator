package controller

import "testing"

// TestEventConstants verifies that event reason constants have the expected string
// values. This guards against accidental renames that would break monitoring
// dashboards and alert rules that depend on stable event reason strings.
func TestEventConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		// Rolling restart lifecycle
		{"RollingRestartStarted", EventRollingRestartStarted, "RollingRestartStarted"},
		{"RollingRestartCompleted", EventRollingRestartCompleted, "RollingRestartCompleted"},
		{"RestartFailed", EventRestartFailed, "RestartFailed"},
		{"PodWarmRestarted", EventPodWarmRestarted, "PodWarmRestarted"},
		{"PodColdRestarted", EventPodColdRestarted, "PodColdRestarted"},
		{"LocalPVCDeleteFailed", EventLocalPVCDeleteFailed, "LocalPVCDeleteFailed"},
		// Quiesce lifecycle
		{"NodeQuiesceStarted", EventNodeQuiesceStarted, "NodeQuiesceStarted"},
		{"NodeQuiesced", EventNodeQuiesced, "NodeQuiesced"},
		{"NodeQuiesceFailed", EventNodeQuiesceFailed, "NodeQuiesceFailed"},
		// Config management
		{"ConfigMapCreated", EventConfigMapCreated, "ConfigMapCreated"},
		{"ConfigMapUpdated", EventConfigMapUpdated, "ConfigMapUpdated"},
		{"DynamicConfigApplied", EventDynamicConfigApplied, "DynamicConfigApplied"},
		{"DynamicConfigStatusFailed", EventDynamicConfigStatusFailed, "DynamicConfigStatusFailed"},
		// StatefulSet / Rack management
		{"StatefulSetCreated", EventStatefulSetCreated, "StatefulSetCreated"},
		{"StatefulSetUpdated", EventStatefulSetUpdated, "StatefulSetUpdated"},
		{"RackScaled", EventRackScaled, "RackScaled"},
		{"ScaleDownDeferred", EventScaleDownDeferred, "ScaleDownDeferred"},
		// ACL synchronization
		{"ACLSyncStarted", EventACLSyncStarted, "ACLSyncStarted"},
		{"ACLSyncCompleted", EventACLSyncCompleted, "ACLSyncCompleted"},
		{"ACLSyncError", EventACLSyncError, "ACLSyncError"},
		// PodDisruptionBudget
		{"PDBCreated", EventPDBCreated, "PDBCreated"},
		{"PDBUpdated", EventPDBUpdated, "PDBUpdated"},
		// Service management
		{"ServiceCreated", EventServiceCreated, "ServiceCreated"},
		{"ServiceUpdated", EventServiceUpdated, "ServiceUpdated"},
		// Cluster lifecycle
		{"ClusterDeletionStarted", EventClusterDeletionStarted, "ClusterDeletionStarted"},
		{"FinalizerRemoved", EventFinalizerRemoved, "FinalizerRemoved"},
		// Template
		{"TemplateApplied", EventTemplateApplied, "TemplateApplied"},
		{"TemplateResolutionError", EventTemplateResolutionError, "TemplateResolutionError"},
		{"TemplateDrifted", EventTemplateDrifted, "TemplateDrifted"},
		// Readiness gate
		{"ReadinessGateSatisfied", EventReadinessGateSatisfied, "ReadinessGateSatisfied"},
		{"ReadinessGateBlocking", EventReadinessGateBlocking, "ReadinessGateBlocking"},
		// PVC cleanup
		{"PVCCleanedUp", EventPVCCleanedUp, "PVCCleanedUp"},
		{"PVCCleanupFailed", EventPVCCleanupFailed, "PVCCleanupFailed"},
		// Circuit breaker
		{"CircuitBreakerActive", EventCircuitBreakerActive, "CircuitBreakerActive"},
		{"CircuitBreakerReset", EventCircuitBreakerReset, "CircuitBreakerReset"},
		// Permanent error
		{"PermanentError", EventPermanentError, "PermanentError"},
		// Miscellaneous
		{"ValidationWarning", EventValidationWarning, "ValidationWarning"},
		{"ReconcileError", EventReconcileError, "ReconcileError"},
		{"Operation", EventOperation, "Operation"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.constant != tc.expected {
				t.Errorf("event constant %s = %q, want %q", tc.name, tc.constant, tc.expected)
			}
		})
	}
}
