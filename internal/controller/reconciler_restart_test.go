package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/podutil"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/utils"
)

func TestGetDirtyVolumes_NilStorage(t *testing.T) {
	result := getDirtyVolumes(nil)
	if result != nil {
		t.Errorf("expected nil for nil storage, got %v", result)
	}
}

func TestGetDirtyVolumes_NoWipeMethod(t *testing.T) {
	storage := &ackov1alpha1.AerospikeStorageSpec{
		Volumes: []ackov1alpha1.VolumeSpec{
			{Name: "data"},
			{Name: "logs"},
		},
	}
	result := getDirtyVolumes(storage)
	if len(result) != 0 {
		t.Errorf("expected empty for volumes without wipe method, got %v", result)
	}
}

func TestGetDirtyVolumes_WithWipeMethod(t *testing.T) {
	storage := &ackov1alpha1.AerospikeStorageSpec{
		Volumes: []ackov1alpha1.VolumeSpec{
			{Name: "data", WipeMethod: ackov1alpha1.VolumeWipeMethodDeleteFiles},
			{Name: "logs"},
			{Name: "index", WipeMethod: ackov1alpha1.VolumeWipeMethodBlkdiscard},
		},
	}
	result := getDirtyVolumes(storage)
	if len(result) != 2 {
		t.Fatalf("expected 2 dirty volumes, got %d: %v", len(result), result)
	}
	if result[0] != "data" || result[1] != "index" {
		t.Errorf("expected [data, index], got %v", result)
	}
}

func TestGetDirtyVolumes_WipeMethodNone(t *testing.T) {
	storage := &ackov1alpha1.AerospikeStorageSpec{
		Volumes: []ackov1alpha1.VolumeSpec{
			{Name: "data", WipeMethod: ackov1alpha1.VolumeWipeMethodNone},
		},
	}
	result := getDirtyVolumes(storage)
	if len(result) != 0 {
		t.Errorf("expected empty for wipe method 'none', got %v", result)
	}
}

func TestGetDirtyVolumes_GlobalPolicy(t *testing.T) {
	storage := &ackov1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &ackov1alpha1.AerospikeVolumePolicy{
			WipeMethod: ackov1alpha1.VolumeWipeMethodDeleteFiles,
		},
		Volumes: []ackov1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: ackov1alpha1.VolumeSource{
					PersistentVolume: &ackov1alpha1.PersistentVolumeSpec{Size: "10Gi"},
				},
			}, // should inherit global filesystem policy
		},
	}
	result := getDirtyVolumes(storage)
	if len(result) != 1 {
		t.Fatalf("expected 1 dirty volume from global policy, got %d: %v", len(result), result)
	}
	if result[0] != "data" {
		t.Errorf("expected [data], got %v", result)
	}
}

func TestPodOrdinal(t *testing.T) {
	tests := []struct {
		name     string
		podName  string
		expected int
	}{
		{"first pod", "sts-0", 0},
		{"second pod", "sts-1", 1},
		{"tenth pod", "sts-9", 9},
		{"double digit", "sts-12", 12},
		{"with rack id", "cluster-1-5", 5},
		{"no dash", "nodash", 0},
		{"non-numeric suffix", "sts-abc", 0},
		{"empty string", "", 0},
		{"trailing dash", "sts-", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := podOrdinal(tc.podName); got != tc.expected {
				t.Errorf("podOrdinal(%q) = %d, want %d", tc.podName, got, tc.expected)
			}
		})
	}
}

func TestDetermineRestartReason(t *testing.T) {
	tests := []struct {
		name               string
		podImage           string
		desiredImage       string
		podConfigHash      string
		desiredConfigHash  string
		podSpecHash        string
		desiredPodSpecHash string
		isWarm             bool
		expected           ackov1alpha1.RestartReason
	}{
		{
			name:         "image changed → ImageChanged",
			podImage:     "aerospike:ce-8.0.0.0",
			desiredImage: "aerospike:ce-8.1.1.1",
			expected:     ackov1alpha1.RestartReasonImageChanged,
		},
		{
			name:               "config hash changed, warm restart → WarmRestart",
			podImage:           "aerospike:ce-8.1.1.1",
			desiredImage:       "aerospike:ce-8.1.1.1",
			podConfigHash:      "old",
			desiredConfigHash:  "new",
			podSpecHash:        "same",
			desiredPodSpecHash: "same",
			isWarm:             true,
			expected:           ackov1alpha1.RestartReasonWarmRestart,
		},
		{
			name:               "config hash changed, cold restart → ConfigChanged",
			podImage:           "aerospike:ce-8.1.1.1",
			desiredImage:       "aerospike:ce-8.1.1.1",
			podConfigHash:      "old",
			desiredConfigHash:  "new",
			podSpecHash:        "same",
			desiredPodSpecHash: "same",
			isWarm:             false,
			expected:           ackov1alpha1.RestartReasonConfigChanged,
		},
		{
			name:               "pod spec hash changed (not image/config) → PodSpecChanged",
			podImage:           "aerospike:ce-8.1.1.1",
			desiredImage:       "aerospike:ce-8.1.1.1",
			podConfigHash:      "same",
			desiredConfigHash:  "same",
			podSpecHash:        "old-spec",
			desiredPodSpecHash: "new-spec",
			isWarm:             false,
			expected:           ackov1alpha1.RestartReasonPodSpecChanged,
		},
		{
			name:               "nothing differs → defaults to ConfigChanged",
			podImage:           "aerospike:ce-8.1.1.1",
			desiredImage:       "aerospike:ce-8.1.1.1",
			podConfigHash:      "same",
			desiredConfigHash:  "same",
			podSpecHash:        "same",
			desiredPodSpecHash: "same",
			isWarm:             false,
			expected:           ackov1alpha1.RestartReasonConfigChanged,
		},
		{
			name:               "image change takes priority over config change",
			podImage:           "aerospike:ce-8.0.0.0",
			desiredImage:       "aerospike:ce-8.1.1.1",
			podConfigHash:      "old",
			desiredConfigHash:  "new",
			podSpecHash:        "old-spec",
			desiredPodSpecHash: "new-spec",
			isWarm:             true,
			expected:           ackov1alpha1.RestartReasonImageChanged,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						utils.ConfigHashAnnotation:  tc.podConfigHash,
						utils.PodSpecHashAnnotation: tc.podSpecHash,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  podutil.AerospikeContainerName,
							Image: tc.podImage,
						},
					},
				},
			}
			got := determineRestartReason(pod, tc.desiredImage, tc.desiredConfigHash, tc.desiredPodSpecHash, tc.isWarm)
			if got != tc.expected {
				t.Errorf("determineRestartReason() = %q, want %q", got, tc.expected)
			}
		})
	}
}

// --- filterUnrestarted tests ---

func TestFilterUnrestarted_AllRestarted(t *testing.T) {
	allPending := []string{"pod-0", "pod-1", "pod-2"}
	podsToRestart := []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-0"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-2"}},
	}

	result := filterUnrestarted(allPending, nil, 3, podsToRestart)
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

func TestFilterUnrestarted_SomeFailed(t *testing.T) {
	allPending := []string{"pod-2", "pod-1", "pod-0"}
	podsToRestart := []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-2"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-0"}},
	}

	// pod-1 failed, pod-2 and pod-0 succeeded
	result := filterUnrestarted(allPending, []string{"pod-1"}, 2, podsToRestart)
	if len(result) != 1 {
		t.Fatalf("expected 1 remaining, got %v", result)
	}
	if result[0] != "pod-1" {
		t.Errorf("expected pod-1 to remain, got %v", result)
	}
}

func TestFilterUnrestarted_AllFailed(t *testing.T) {
	allPending := []string{"pod-1", "pod-0"}
	podsToRestart := []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-0"}},
	}

	result := filterUnrestarted(allPending, []string{"pod-1", "pod-0"}, 0, podsToRestart)
	if len(result) != 2 {
		t.Fatalf("expected 2 remaining, got %v", result)
	}
}

func TestFilterUnrestarted_WithUnattemptedPods(t *testing.T) {
	// 5 pending, only 2 attempted (batch size), 1 succeeded, 1 failed
	allPending := []string{"pod-4", "pod-3", "pod-2", "pod-1", "pod-0"}
	podsToRestart := []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-4"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-3"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-2"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-0"}},
	}

	// pod-4 succeeded, pod-3 failed, rest not attempted
	result := filterUnrestarted(allPending, []string{"pod-3"}, 1, podsToRestart)
	// Should contain: pod-3 (failed), pod-2, pod-1, pod-0 (not attempted)
	if len(result) != 4 {
		t.Fatalf("expected 4 remaining, got %v", result)
	}
	// pod-4 should not be in result (it was restarted successfully)
	for _, name := range result {
		if name == "pod-4" {
			t.Error("pod-4 was successfully restarted, should not be in remaining")
		}
	}
}

func TestFilterUnrestarted_EmptyInputs(t *testing.T) {
	result := filterUnrestarted(nil, nil, 0, nil)
	if len(result) != 0 {
		t.Errorf("expected empty for nil inputs, got %v", result)
	}
}

// TestFilterUnrestarted_BatchSubsetSemantics locks in the new contract: callers
// must pass the actual batch slice, not the full pending queue. Previously the
// caller passed `podsToRestart` (full queue), which meant pods beyond the batch
// boundary could be incorrectly classified as restarted whenever `restarted`
// happened to count past the real batch — an invariant that broke as soon as
// dynamic-config returned len(batchPods) but the queue contained more pods.
func TestFilterUnrestarted_BatchSubsetSemantics(t *testing.T) {
	allPending := []string{"pod-4", "pod-3", "pod-2", "pod-1", "pod-0"}

	// Only pod-4 and pod-3 were actually attempted in this reconcile pass
	// (batchSize=2). Both succeeded.
	batchPods := []*corev1.Pod{
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-4"}},
		{ObjectMeta: metav1.ObjectMeta{Name: "pod-3"}},
	}

	result := filterUnrestarted(allPending, nil, 2, batchPods)

	// pod-2, pod-1, pod-0 were never attempted and must remain pending.
	if len(result) != 3 {
		t.Fatalf("expected 3 remaining (the unattempted tail), got %v", result)
	}
	want := map[string]bool{"pod-2": true, "pod-1": true, "pod-0": true}
	for _, name := range result {
		if !want[name] {
			t.Errorf("unexpected pod in remaining: %q", name)
		}
	}
}
