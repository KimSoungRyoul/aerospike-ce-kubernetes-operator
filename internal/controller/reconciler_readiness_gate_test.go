package controller

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/podutil"
)

// ---------- findPodReadinessCondition ----------

func TestFindPodReadinessCondition(t *testing.T) {
	tests := []struct {
		name       string
		conditions []corev1.PodCondition
		wantSat    bool
		wantExists bool
	}{
		{
			name:       "no conditions",
			conditions: nil,
			wantSat:    false,
			wantExists: false,
		},
		{
			name: "gate condition True",
			conditions: []corev1.PodCondition{
				{Type: podutil.AerospikeReadinessGateConditionType, Status: corev1.ConditionTrue},
			},
			wantSat:    true,
			wantExists: true,
		},
		{
			name: "gate condition False",
			conditions: []corev1.PodCondition{
				{Type: podutil.AerospikeReadinessGateConditionType, Status: corev1.ConditionFalse},
			},
			wantSat:    false,
			wantExists: true,
		},
		{
			name: "other conditions but no gate",
			conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				{Type: corev1.ContainersReady, Status: corev1.ConditionTrue},
			},
			wantSat:    false,
			wantExists: false,
		},
		{
			name: "gate among other conditions",
			conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				{Type: podutil.AerospikeReadinessGateConditionType, Status: corev1.ConditionTrue},
			},
			wantSat:    true,
			wantExists: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				Status: corev1.PodStatus{Conditions: tt.conditions},
			}
			sat, exists := findPodReadinessCondition(pod)
			if sat != tt.wantSat {
				t.Errorf("satisfied: got %v, want %v", sat, tt.wantSat)
			}
			if exists != tt.wantExists {
				t.Errorf("exists: got %v, want %v", exists, tt.wantExists)
			}
		})
	}
}

// ---------- upsertPodCondition ----------

func TestUpsertPodCondition(t *testing.T) {
	t.Run("appends new condition when empty", func(t *testing.T) {
		pod := &corev1.Pod{}
		cond := corev1.PodCondition{
			Type:   podutil.AerospikeReadinessGateConditionType,
			Status: corev1.ConditionTrue,
		}
		upsertPodCondition(pod, cond)
		if len(pod.Status.Conditions) != 1 {
			t.Fatalf("expected 1 condition, got %d", len(pod.Status.Conditions))
		}
		if pod.Status.Conditions[0].Status != corev1.ConditionTrue {
			t.Errorf("status: got %v, want True", pod.Status.Conditions[0].Status)
		}
	})

	t.Run("updates existing condition and changes LastTransitionTime on status change", func(t *testing.T) {
		earlier := metav1.NewTime(time.Now().Add(-1 * time.Hour))
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{
						Type:               podutil.AerospikeReadinessGateConditionType,
						Status:             corev1.ConditionFalse,
						LastTransitionTime: earlier,
					},
				},
			},
		}
		newCond := corev1.PodCondition{
			Type:               podutil.AerospikeReadinessGateConditionType,
			Status:             corev1.ConditionTrue, // status changed
			LastTransitionTime: metav1.Now(),
		}
		upsertPodCondition(pod, newCond)
		if len(pod.Status.Conditions) != 1 {
			t.Fatalf("expected 1 condition, got %d", len(pod.Status.Conditions))
		}
		got := pod.Status.Conditions[0]
		if got.Status != corev1.ConditionTrue {
			t.Errorf("status: got %v, want True", got.Status)
		}
		if got.LastTransitionTime.Equal(&earlier) {
			t.Errorf("LastTransitionTime should have been updated but was preserved")
		}
	})

	t.Run("preserves LastTransitionTime when status unchanged", func(t *testing.T) {
		earlier := metav1.NewTime(time.Now().Add(-1 * time.Hour))
		pod := &corev1.Pod{
			Status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{
					{
						Type:               podutil.AerospikeReadinessGateConditionType,
						Status:             corev1.ConditionTrue,
						LastTransitionTime: earlier,
					},
				},
			},
		}
		newCond := corev1.PodCondition{
			Type:               podutil.AerospikeReadinessGateConditionType,
			Status:             corev1.ConditionTrue, // same status
			LastTransitionTime: metav1.Now(),
		}
		upsertPodCondition(pod, newCond)
		got := pod.Status.Conditions[0]
		if !got.LastTransitionTime.Equal(&earlier) {
			t.Errorf("LastTransitionTime should be preserved when status unchanged")
		}
	})
}

// ---------- isReadinessGateEnabled ----------

func TestIsReadinessGateEnabled(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name    string
		podSpec *ackov1alpha1.AerospikePodSpec
		want    bool
	}{
		{"nil podSpec", nil, false},
		{"nil ReadinessGateEnabled", &ackov1alpha1.AerospikePodSpec{}, false},
		{"ReadinessGateEnabled=false", &ackov1alpha1.AerospikePodSpec{ReadinessGateEnabled: &falseVal}, false},
		{"ReadinessGateEnabled=true", &ackov1alpha1.AerospikePodSpec{ReadinessGateEnabled: &trueVal}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &ackov1alpha1.AerospikeCluster{
				Spec: ackov1alpha1.AerospikeClusterSpec{PodSpec: tt.podSpec},
			}
			if got := isReadinessGateEnabled(cluster); got != tt.want {
				t.Errorf("isReadinessGateEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------- isPodReadinessGateSatisfied ----------

func TestIsPodReadinessGateSatisfied(t *testing.T) {
	trueVal := true

	clusterEnabled := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			PodSpec: &ackov1alpha1.AerospikePodSpec{ReadinessGateEnabled: &trueVal},
		},
	}
	clusterDisabled := &ackov1alpha1.AerospikeCluster{}

	podWithGateTrue := &corev1.Pod{
		Spec: corev1.PodSpec{
			ReadinessGates: []corev1.PodReadinessGate{
				{ConditionType: podutil.AerospikeReadinessGateConditionType},
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{Type: podutil.AerospikeReadinessGateConditionType, Status: corev1.ConditionTrue},
			},
		},
	}
	podWithGateFalse := &corev1.Pod{
		Spec: corev1.PodSpec{
			ReadinessGates: []corev1.PodReadinessGate{
				{ConditionType: podutil.AerospikeReadinessGateConditionType},
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{Type: podutil.AerospikeReadinessGateConditionType, Status: corev1.ConditionFalse},
			},
		},
	}
	podWithGateNoCondition := &corev1.Pod{
		Spec: corev1.PodSpec{
			ReadinessGates: []corev1.PodReadinessGate{
				{ConditionType: podutil.AerospikeReadinessGateConditionType},
			},
		},
	}
	podWithoutGate := &corev1.Pod{}

	tests := []struct {
		name    string
		cluster *ackov1alpha1.AerospikeCluster
		pod     *corev1.Pod
		want    bool
	}{
		{"feature disabled → always true", clusterDisabled, podWithGateFalse, true},
		{"feature enabled, gate True → true", clusterEnabled, podWithGateTrue, true},
		{"feature enabled, gate False → false", clusterEnabled, podWithGateFalse, false},
		{"feature enabled, no condition → false", clusterEnabled, podWithGateNoCondition, false},
		{"feature enabled, pod predates feature (no gate in spec) → true", clusterEnabled, podWithoutGate, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPodReadinessGateSatisfied(tt.cluster, tt.pod); got != tt.want {
				t.Errorf("isPodReadinessGateSatisfied() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ---------- anyPodGateUnsatisfied ----------

func TestAnyPodGateUnsatisfied(t *testing.T) {
	trueVal := true
	clusterEnabled := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			PodSpec: &ackov1alpha1.AerospikePodSpec{ReadinessGateEnabled: &trueVal},
		},
	}

	readyPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-0"},
		Spec: corev1.PodSpec{
			ReadinessGates: []corev1.PodReadinessGate{
				{ConditionType: podutil.AerospikeReadinessGateConditionType},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: podutil.AerospikeReadinessGateConditionType, Status: corev1.ConditionTrue},
			},
		},
	}
	notReadyPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-1"},
		Spec: corev1.PodSpec{
			ReadinessGates: []corev1.PodReadinessGate{
				{ConditionType: podutil.AerospikeReadinessGateConditionType},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: podutil.AerospikeReadinessGateConditionType, Status: corev1.ConditionFalse},
			},
		},
	}
	pendingPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-2"},
		Spec: corev1.PodSpec{
			ReadinessGates: []corev1.PodReadinessGate{
				{ConditionType: podutil.AerospikeReadinessGateConditionType},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}
	now := metav1.Now()
	terminatingPod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-3", DeletionTimestamp: &now},
		Spec: corev1.PodSpec{
			ReadinessGates: []corev1.PodReadinessGate{
				{ConditionType: podutil.AerospikeReadinessGateConditionType},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	tests := []struct {
		name      string
		pods      []corev1.Pod
		wantBlock bool
		wantPod   string
	}{
		{"empty rack → not blocked", nil, false, ""},
		{"all pods gate satisfied → not blocked", []corev1.Pod{readyPod}, false, ""},
		{"one pod gate unsatisfied → blocked", []corev1.Pod{readyPod, notReadyPod}, true, "pod-1"},
		{"pending pod skipped", []corev1.Pod{pendingPod}, false, ""},
		{"terminating pod skipped", []corev1.Pod{terminatingPod}, false, ""},
		{"notReady + pending → blocked on notReady", []corev1.Pod{pendingPod, notReadyPod}, true, "pod-1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocked, podName := anyPodGateUnsatisfied(clusterEnabled, tt.pods)
			if blocked != tt.wantBlock {
				t.Errorf("blocked: got %v, want %v", blocked, tt.wantBlock)
			}
			if podName != tt.wantPod {
				t.Errorf("podName: got %q, want %q", podName, tt.wantPod)
			}
		})
	}
}
