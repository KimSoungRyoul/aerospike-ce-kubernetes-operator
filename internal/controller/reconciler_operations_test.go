package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

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

	done := finalizeOperationPhase(opStatus, pods, completed)
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
	// Both pods are "completed" in the sense that we've stopped retrying them,
	// but one of them ended up on the FailedPods list.
	completed := map[string]bool{"pod-0": true, "pod-1": true}
	opStatus := &ackov1alpha1.OperationStatus{
		Phase:      ackov1alpha1.AerospikePhaseInProgress,
		FailedPods: []string{"pod-1"},
	}

	done := finalizeOperationPhase(opStatus, pods, completed)
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

	done := finalizeOperationPhase(opStatus, pods, completed)
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

	done := finalizeOperationPhase(opStatus, pods, completed)
	if !done {
		t.Fatal("regression: phase stuck InProgress after all pods completed")
	}
	if opStatus.Phase != ackov1alpha1.AerospikePhaseCompleted {
		t.Errorf("regression: phase = %q, want %q", opStatus.Phase, ackov1alpha1.AerospikePhaseCompleted)
	}
}

func TestFinalizeOperationPhase_NoPods_IsCompleted(t *testing.T) {
	opStatus := &ackov1alpha1.OperationStatus{Phase: ackov1alpha1.AerospikePhaseInProgress}
	done := finalizeOperationPhase(opStatus, nil, map[string]bool{})
	if !done {
		t.Fatal("expected done=true with no target pods")
	}
	if opStatus.Phase != ackov1alpha1.AerospikePhaseCompleted {
		t.Errorf("phase = %q, want %q", opStatus.Phase, ackov1alpha1.AerospikePhaseCompleted)
	}
}
