package controller

// Event reason constants for Kubernetes Events recorded by the AerospikeCluster controller.
// Use these constants instead of hardcoded strings to avoid typos and enable consistent monitoring.
const (
	// Rolling restart lifecycle
	EventRollingRestartStarted   = "RollingRestartStarted"
	EventRollingRestartDeferred  = "RollingRestartDeferred"
	EventRollingRestartCompleted = "RollingRestartCompleted"
	EventRestartFailed           = "RestartFailed"
	EventPodWarmRestarted        = "PodWarmRestarted"
	EventPodColdRestarted        = "PodColdRestarted"
	EventLocalPVCDeleteFailed    = "LocalPVCDeleteFailed"

	// Quiesce lifecycle
	EventNodeQuiesceStarted = "NodeQuiesceStarted"
	EventNodeQuiesced       = "NodeQuiesced"
	EventNodeQuiesceFailed  = "NodeQuiesceFailed"

	// Config management
	EventConfigMapCreated          = "ConfigMapCreated"
	EventConfigMapUpdated          = "ConfigMapUpdated"
	EventDynamicConfigApplied      = "DynamicConfigApplied"
	EventDynamicConfigStatusFailed = "DynamicConfigStatusFailed"
	EventDynamicConfigDegraded     = "DynamicConfigDegraded"
	EventDynamicConfigRollback     = "DynamicConfigRollback"
	EventConfigDegradedSkip        = "ConfigDegradedSkip"

	// StatefulSet / Rack management
	EventStatefulSetCreated = "StatefulSetCreated"
	EventStatefulSetUpdated = "StatefulSetUpdated"
	EventRackScaled         = "RackScaled"
	EventScaleDownDeferred  = "ScaleDownDeferred"

	// ACL synchronization
	EventACLSyncStarted   = "ACLSyncStarted"
	EventACLSyncCompleted = "ACLSyncCompleted"
	EventACLSyncError     = "ACLSyncError"

	// PodDisruptionBudget
	EventPDBCreated = "PDBCreated"
	EventPDBUpdated = "PDBUpdated"

	// Service management
	EventServiceCreated = "ServiceCreated"
	EventServiceUpdated = "ServiceUpdated"
	EventServiceDeleted = "ServiceDeleted"

	// Pause/Resume lifecycle
	EventPaused  = "ReconciliationPaused"
	EventResumed = "ReconciliationResumed"

	// Cluster lifecycle
	EventClusterDeletionStarted = "ClusterDeletionStarted"
	EventFinalizerRemoved       = "FinalizerRemoved"

	// Template
	EventTemplateApplied         = "TemplateApplied"
	EventTemplateResolutionError = "TemplateResolutionError"
	EventTemplateDrifted         = "TemplateDrifted"

	// Readiness gate
	EventReadinessGateSatisfied = "ReadinessGateSatisfied"
	EventReadinessGateBlocking  = "ReadinessGateBlocking"

	// Migration checks
	EventMigrationCheckFailed = "MigrationCheckFailed"

	// PVC cleanup
	EventPVCCleanedUp     = "PVCCleanedUp"
	EventPVCCleanupFailed = "PVCCleanupFailed"

	// Circuit breaker
	EventCircuitBreakerActive = "CircuitBreakerActive"
	EventCircuitBreakerReset  = "CircuitBreakerReset"

	// Permanent error
	EventPermanentError = "PermanentError"

	// Miscellaneous
	EventValidationWarning = "ValidationWarning"
	EventReconcileError    = "ReconcileError"
	EventOperation         = "Operation"
)
