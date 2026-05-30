package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/podutil"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/utils"
)

// TestReconcileOperations_ExplicitPodListNoMatch_GoesToError is the regression
// guard for the silent-no-op bug: an operation that names pods in podList which
// do not exist (typo / stale / deleted) previously resolved to an empty target
// set, and finalizeOperationPhase then marked it Completed having restarted
// nothing. The operator should instead see the Error phase plus a Warning event.
func TestReconcileOperations_ExplicitPodListNoMatch_GoesToError(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := ackov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("corev1 AddToScheme() error = %v", err)
	}

	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size: 1,
			Operations: []ackov1alpha1.OperationSpec{
				{
					ID:      "op-typo",
					Kind:    ackov1alpha1.OperationWarmRestart,
					PodList: []string{"demo-does-not-exist"},
				},
			},
		},
	}

	// A real pod exists in the cluster, but it is NOT the one named in podList.
	// This proves the guard keys on "named pods unresolved", not "cluster empty".
	existingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo-0",
			Namespace: "default",
			Labels:    utils.SelectorLabelsForCluster("demo"),
		},
	}

	recorder := record.NewFakeRecorder(8)
	reconciler := &AerospikeClusterReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&ackov1alpha1.AerospikeCluster{}).
			WithObjects(cluster, existingPod).
			Build(),
		Scheme:   scheme,
		Recorder: recorder,
	}

	inProgress, err := reconciler.reconcileOperations(context.Background(), cluster)
	if err != nil {
		t.Fatalf("reconcileOperations() error = %v", err)
	}
	if inProgress {
		t.Fatal("expected inProgress=false: an unresolved-target operation is terminal, not requeued")
	}

	updated := &ackov1alpha1.AerospikeCluster{}
	if err := reconciler.Get(context.Background(), types.NamespacedName{Name: "demo", Namespace: "default"}, updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.OperationStatus == nil {
		t.Fatal("expected OperationStatus to be set")
	}
	if updated.Status.OperationStatus.Phase != ackov1alpha1.AerospikePhaseError {
		t.Errorf("phase = %q, want %q (unresolved named pods must not be reported as Completed)",
			updated.Status.OperationStatus.Phase, ackov1alpha1.AerospikePhaseError)
	}
	if len(updated.Status.OperationStatus.CompletedPods) != 0 {
		t.Errorf("CompletedPods = %v, want empty (nothing was actually restarted)",
			updated.Status.OperationStatus.CompletedPods)
	}

	// A Warning event must have been emitted to make the misconfiguration visible.
	select {
	case ev := <-recorder.Events:
		if ev == "" {
			t.Error("expected a non-empty Warning event")
		}
	default:
		t.Error("expected a Warning event for the unresolved operation target")
	}

	// Idempotency: a second reconcile with the same (now Error) status must
	// short-circuit and not flip the phase or requeue.
	inProgress2, err := reconciler.reconcileOperations(context.Background(), updated)
	if err != nil {
		t.Fatalf("second reconcileOperations() error = %v", err)
	}
	if inProgress2 {
		t.Error("expected inProgress=false on the second reconcile (Error phase is terminal)")
	}
}

func TestFilterPodsByNames_EmptyNames_ReturnsAll(t *testing.T) {
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-0"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-2"}},
	}

	result := filterPodsByNames(pods, nil)
	if len(result) != 3 {
		t.Fatalf("expected 3 pods, got %d", len(result))
	}
	for i, p := range result {
		if p.Name != pods[i].Name {
			t.Errorf("pod[%d] = %q, want %q", i, p.Name, pods[i].Name)
		}
	}
}

func TestFilterPodsByNames_SpecificNames(t *testing.T) {
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-0"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-2"}},
	}

	result := filterPodsByNames(pods, []string{"pod-0", "pod-2"})
	if len(result) != 2 {
		t.Fatalf("expected 2 pods, got %d", len(result))
	}
	if result[0].Name != "pod-0" {
		t.Errorf("result[0] = %q, want %q", result[0].Name, "pod-0")
	}
	if result[1].Name != "pod-2" {
		t.Errorf("result[1] = %q, want %q", result[1].Name, "pod-2")
	}
}

func TestFilterPodsByNames_NonExistentNames(t *testing.T) {
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-0"}},
	}

	result := filterPodsByNames(pods, []string{"pod-99", "missing"})
	if len(result) != 0 {
		t.Fatalf("expected 0 pods for non-existent names, got %d", len(result))
	}
}

func TestFilterPodsByNames_MixedExistAndMissing(t *testing.T) {
	pods := []corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-0"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}},
	}

	result := filterPodsByNames(pods, []string{"pod-1", "nonexistent"})
	if len(result) != 1 {
		t.Fatalf("expected 1 pod, got %d", len(result))
	}
	if result[0].Name != "pod-1" {
		t.Errorf("result[0] = %q, want %q", result[0].Name, "pod-1")
	}
}

func TestFilterPodsByNames_EmptyPodList(t *testing.T) {
	result := filterPodsByNames(nil, []string{"pod-0"})
	if len(result) != 0 {
		t.Fatalf("expected 0 pods for empty pod list, got %d", len(result))
	}
}

func TestFilterPodsByNames_EmptyBoth(t *testing.T) {
	result := filterPodsByNames(nil, nil)
	if len(result) != 0 {
		t.Fatalf("expected 0 pods for empty inputs, got %d", len(result))
	}
}

// --- finalizeOperationPhase tests ---
//
// Regression coverage for the "allDone" bug: previously the phase was tracked
// via a bool that flipped to false on the first incomplete pod and was never
// restored, so a multi-reconcile operation would stay InProgress forever even
// after every target pod had been processed. These cases lock in the new
// "count remaining pods" logic.

func TestFinalizeOperationPhase_AllCompleted_NoFailures(t *testing.T) {
	pods := []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-0"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-2"}},
	}
	completed := map[string]bool{"pod-0": true, "pod-1": true, "pod-2": true}
	opStatus := &ackov1alpha1.OperationStatus{Phase: ackov1alpha1.AerospikePhaseInProgress}

	done := finalizeOperationPhase(opStatus, pods, completed, map[string]bool{})
	if !done {
		t.Fatal("expected done=true when every pod is in completedSet")
	}
	if opStatus.Phase != ackov1alpha1.AerospikePhaseCompleted {
		t.Errorf("phase = %q, want %q", opStatus.Phase, ackov1alpha1.AerospikePhaseCompleted)
	}
}

func TestFinalizeOperationPhase_AllCompleted_WithFailures_GoesToError(t *testing.T) {
	pods := []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-0"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}},
	}
	// pod-0 succeeded; pod-1 was attempted but failed. A failed pod counts as
	// resolved, so nothing is outstanding and the operation must terminate.
	completed := map[string]bool{"pod-0": true}
	failed := map[string]bool{"pod-1": true}
	opStatus := &ackov1alpha1.OperationStatus{
		Phase:      ackov1alpha1.AerospikePhaseInProgress,
		FailedPods: []string{"pod-1"},
	}

	done := finalizeOperationPhase(opStatus, pods, completed, failed)
	if !done {
		t.Fatal("expected done=true when nothing is outstanding")
	}
	if opStatus.Phase != ackov1alpha1.AerospikePhaseError {
		t.Errorf("phase = %q, want %q (FailedPods should drive Error phase)",
			opStatus.Phase, ackov1alpha1.AerospikePhaseError)
	}
}

func TestFinalizeOperationPhase_PartialProgress_StaysInProgress(t *testing.T) {
	pods := []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-0"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-2"}},
	}
	completed := map[string]bool{"pod-0": true} // pod-1, pod-2 still pending
	opStatus := &ackov1alpha1.OperationStatus{Phase: ackov1alpha1.AerospikePhaseInProgress}

	done := finalizeOperationPhase(opStatus, pods, completed, map[string]bool{})
	if done {
		t.Fatal("expected done=false while pods remain outstanding")
	}
	if opStatus.Phase != ackov1alpha1.AerospikePhaseInProgress {
		t.Errorf("phase = %q, want %q (must stay InProgress until everything is done)",
			opStatus.Phase, ackov1alpha1.AerospikePhaseInProgress)
	}
}

// TestFinalizeOperationPhase_RecoversFromInterleavedProgress is the direct
// regression for the original allDone bug: the loop in reconcileOperations
// processes pods in order. The first iteration finds pod-0 already completed
// (continue), the second hits pod-1 outstanding (old code: allDone=false
// permanently). The completion check must look at the *final* state, not the
// transient mid-loop state.
func TestFinalizeOperationPhase_RecoversFromInterleavedProgress(t *testing.T) {
	pods := []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-0"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}},
	}
	// Simulate the state right after the loop finished processing the last
	// outstanding pod: every pod is now in completedSet.
	completed := map[string]bool{"pod-0": true, "pod-1": true}
	opStatus := &ackov1alpha1.OperationStatus{Phase: ackov1alpha1.AerospikePhaseInProgress}

	done := finalizeOperationPhase(opStatus, pods, completed, map[string]bool{})
	if !done {
		t.Fatal("regression: phase stuck InProgress after all pods completed")
	}
	if opStatus.Phase != ackov1alpha1.AerospikePhaseCompleted {
		t.Errorf("regression: phase = %q, want %q", opStatus.Phase, ackov1alpha1.AerospikePhaseCompleted)
	}
}

func TestFinalizeOperationPhase_NoPods_IsCompleted(t *testing.T) {
	opStatus := &ackov1alpha1.OperationStatus{Phase: ackov1alpha1.AerospikePhaseInProgress}
	done := finalizeOperationPhase(opStatus, nil, map[string]bool{}, map[string]bool{})
	if !done {
		t.Fatal("expected done=true with no target pods")
	}
	if opStatus.Phase != ackov1alpha1.AerospikePhaseCompleted {
		t.Errorf("phase = %q, want %q", opStatus.Phase, ackov1alpha1.AerospikePhaseCompleted)
	}
}

// TestFinalizeOperationPhase_AllFailed_ReachesTerminalError is the direct
// regression for the infinite-reconcile bug: every target pod failed its
// warm/cold restart. Before the fix, failed pods were absent from completedSet
// so remainingPods stayed > 0, the operation never left InProgress, and the
// cluster requeued every 5s forever. A failed pod must now count as resolved
// so the operation reaches the terminal Error phase.
func TestFinalizeOperationPhase_AllFailed_ReachesTerminalError(t *testing.T) {
	pods := []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-0"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}},
	}
	completed := map[string]bool{}
	failed := map[string]bool{"pod-0": true, "pod-1": true}
	opStatus := &ackov1alpha1.OperationStatus{
		Phase:      ackov1alpha1.AerospikePhaseInProgress,
		FailedPods: []string{"pod-0", "pod-1"},
	}

	done := finalizeOperationPhase(opStatus, pods, completed, failed)
	if !done {
		t.Fatal("regression: operation never terminates when every pod fails (infinite reconcile loop)")
	}
	if opStatus.Phase != ackov1alpha1.AerospikePhaseError {
		t.Errorf("phase = %q, want %q (a fully-failed operation must reach terminal Error)",
			opStatus.Phase, ackov1alpha1.AerospikePhaseError)
	}
}

// TestFinalizeOperationPhase_FailedPodNotRetriedKeepsOutstanding verifies that
// while one pod has failed (resolved) another genuinely pending pod still keeps
// the operation InProgress — i.e. a failed pod neither blocks completion nor
// masks remaining work.
func TestFinalizeOperationPhase_FailedPodNotRetriedKeepsOutstanding(t *testing.T) {
	pods := []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-0"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}},
	}
	completed := map[string]bool{}
	failed := map[string]bool{"pod-0": true} // pod-1 not yet attempted
	opStatus := &ackov1alpha1.OperationStatus{Phase: ackov1alpha1.AerospikePhaseInProgress}

	done := finalizeOperationPhase(opStatus, pods, completed, failed)
	if done {
		t.Fatal("expected done=false: pod-1 is still outstanding")
	}
	if opStatus.Phase != ackov1alpha1.AerospikePhaseInProgress {
		t.Errorf("phase = %q, want %q", opStatus.Phase, ackov1alpha1.AerospikePhaseInProgress)
	}
}

// TestOperationFailedPodsDedup simulates the cross-reconcile dedup logic from
// reconcileOperations: a pod already on FailedPods from a previous reconcile
// must not be appended again, otherwise FailedPods grows unbounded with
// duplicates on every 5s requeue.
func TestOperationFailedPodsDedup(t *testing.T) {
	// Prior status carried forward into a fresh opStatus, as reconcileOperations does.
	prev := &ackov1alpha1.OperationStatus{FailedPods: []string{"pod-0"}}
	opStatus := &ackov1alpha1.OperationStatus{FailedPods: prev.FailedPods}

	failedSet := make(map[string]bool)
	for _, p := range prev.FailedPods {
		failedSet[p] = true
	}

	// Re-observe pod-0 failing on the next reconcile.
	podName := "pod-0"
	if !failedSet[podName] {
		opStatus.FailedPods = append(opStatus.FailedPods, podName)
		failedSet[podName] = true
	}

	if len(opStatus.FailedPods) != 1 {
		t.Fatalf("FailedPods = %v, want exactly one entry (no duplicates across reconciles)", opStatus.FailedPods)
	}
}

// operationsScheme builds a scheme with both acko and core/v1 types registered,
// as the controller-level operations tests need.
func operationsScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := ackov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme(acko) error = %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("corev1 AddToScheme() error = %v", err)
	}
	return scheme
}

// clusterPod builds a pod carrying the cluster selector labels so listClusterPods
// (and thus getOperationTargetPods) resolves it as an operation target.
func clusterPod(clusterName, name string, phase corev1.PodPhase) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels:    utils.SelectorLabelsForCluster(clusterName),
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: podutil.AerospikeContainerName, Image: "aerospike:ce-8.1.1.1"},
			},
		},
		Status: corev1.PodStatus{Phase: phase},
	}
}

// TestReconcileOperations_BlockedByReadinessGate_DoesNotAdvance is the core
// data-availability regression for Issue 1: on-demand operations must honor the
// same readiness/migration guard the rolling-restart path uses (isBatchBlocked),
// so a batched PodRestart cannot take a second node down while a previously
// restarted node is still cold/migrating. Here a previously restarted pod has an
// unsatisfied readiness gate. reconcileOperations must requeue (inProgress=true)
// WITHOUT restarting any further pod or advancing CompletedPods.
//
// The readiness-gate path is the in-memory test seam (isReadinessGateEnabled +
// anyPodGateUnsatisfied); it keeps isBatchBlocked off the network-bound
// migration probe, mirroring how reconciler_restart_rolling_test.go exercises
// the same guard.
func TestReconcileOperations_BlockedByReadinessGate_DoesNotAdvance(t *testing.T) {
	scheme := operationsScheme(t)
	gateEnabled := true

	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size:    2,
			PodSpec: &ackov1alpha1.AerospikePodSpec{ReadinessGateEnabled: &gateEnabled},
			Operations: []ackov1alpha1.OperationSpec{
				{
					ID:   "op-rolling",
					Kind: ackov1alpha1.OperationPodRestart,
					// Empty PodList → operation targets every cluster pod.
				},
			},
		},
	}

	// demo-0: already restarted, but its readiness gate is NOT yet satisfied
	// (Running + gate condition False) → isBatchBlocked must hold the batch.
	blockedPod := clusterPod("demo", "demo-0", corev1.PodRunning)
	blockedPod.Spec.ReadinessGates = []corev1.PodReadinessGate{
		{ConditionType: podutil.AerospikeReadinessGateConditionType},
	}
	blockedPod.Status.Conditions = []corev1.PodCondition{
		{Type: podutil.AerospikeReadinessGateConditionType, Status: corev1.ConditionFalse},
	}
	// demo-1: the next pod the operation would restart if the guard were absent.
	nextPod := clusterPod("demo", "demo-1", corev1.PodRunning)

	reconciler := &AerospikeClusterReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&ackov1alpha1.AerospikeCluster{}).
			WithObjects(cluster, blockedPod, nextPod).
			Build(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(16),
	}

	inProgress, err := reconciler.reconcileOperations(context.Background(), cluster)
	if err != nil {
		t.Fatalf("reconcileOperations() error = %v", err)
	}
	if !inProgress {
		t.Fatal("expected inProgress=true: a blocked batch must requeue, not complete")
	}

	// No pod should have been restarted: demo-1 must still exist.
	if err := reconciler.Get(context.Background(),
		types.NamespacedName{Name: "demo-1", Namespace: "default"}, &corev1.Pod{}); err != nil {
		t.Fatalf("demo-1 should NOT have been restarted while the batch is blocked, Get err = %v", err)
	}

	// CompletedPods must not have advanced while blocked.
	updated := &ackov1alpha1.AerospikeCluster{}
	if err := reconciler.Get(context.Background(),
		types.NamespacedName{Name: "demo", Namespace: "default"}, updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.OperationStatus != nil && len(updated.Status.OperationStatus.CompletedPods) != 0 {
		t.Errorf("CompletedPods = %v, want empty while batch is blocked",
			updated.Status.OperationStatus.CompletedPods)
	}
}

// TestReconcileOperations_NotBlocked_AdvancesOneBatch is the companion
// regression: with nothing blocking (readiness gate enabled, no pod
// gate-unsatisfied), reconcileOperations advances exactly one batch (batchSize
// defaults to 1) — the targeted pod is cold-restarted (deleted) and marked
// completed. This proves the new guard does not over-block the happy path.
//
// The target pod is Pending (not ready) so coldRestartPod skips the best-effort
// quiesce network call, and a Pending pod is skipped by anyPodGateUnsatisfied so
// it does not block itself.
func TestReconcileOperations_NotBlocked_AdvancesOneBatch(t *testing.T) {
	scheme := operationsScheme(t)
	gateEnabled := true

	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "default"},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size:    1,
			PodSpec: &ackov1alpha1.AerospikePodSpec{ReadinessGateEnabled: &gateEnabled},
			Operations: []ackov1alpha1.OperationSpec{
				{
					ID:      "op-single",
					Kind:    ackov1alpha1.OperationPodRestart,
					PodList: []string{"demo-0"},
				},
			},
		},
	}

	target := clusterPod("demo", "demo-0", corev1.PodPending)

	reconciler := &AerospikeClusterReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&ackov1alpha1.AerospikeCluster{}).
			WithObjects(cluster, target).
			Build(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(16),
	}

	_, err := reconciler.reconcileOperations(context.Background(), cluster)
	if err != nil {
		t.Fatalf("reconcileOperations() error = %v", err)
	}

	// Cold restart deletes the pod.
	if err := reconciler.Get(context.Background(),
		types.NamespacedName{Name: "demo-0", Namespace: "default"}, &corev1.Pod{}); err == nil {
		t.Fatal("expected demo-0 to be deleted (cold restart) when the batch is not blocked")
	}

	updated := &ackov1alpha1.AerospikeCluster{}
	if err := reconciler.Get(context.Background(),
		types.NamespacedName{Name: "demo", Namespace: "default"}, updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if updated.Status.OperationStatus == nil {
		t.Fatal("expected OperationStatus to be set")
	}
	if len(updated.Status.OperationStatus.CompletedPods) != 1 ||
		updated.Status.OperationStatus.CompletedPods[0] != "demo-0" {
		t.Errorf("CompletedPods = %v, want [demo-0] (one batch advanced)",
			updated.Status.OperationStatus.CompletedPods)
	}
}
