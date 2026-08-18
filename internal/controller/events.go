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
	// EventPDBNameConflict is emitted when a PodDisruptionBudget with the name
	// this cluster wants already carries another cluster's instance label. PDB
	// names can collide across clusters in one namespace — RackPDBName("demo", 1)
	// and PDBName("demo-1") are both "demo-1-pdb".
	EventPDBNameConflict = "PDBNameConflict"

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
	// EventMigrationCheckUnavailable is emitted when the rolling restart proceeds
	// WITHOUT a confirmed migration answer, after the bounded escape hatch in
	// isBatchBlocked opens. Distinct from EventMigrationCheckFailed (which means
	// the batch was held) because this is the only path that deletes pods with
	// migration state unknown.
	EventMigrationCheckUnavailable = "MigrationCheckUnavailable"

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
