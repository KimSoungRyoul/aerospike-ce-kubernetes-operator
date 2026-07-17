package controller

import (
	"context"
	"strings"
	"testing"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func TestGetRacks(t *testing.T) {
	r := &AerospikeClusterReconciler{}

	tests := []struct {
		name    string
		cluster *ackov1alpha1.AerospikeCluster
		wantLen int
		wantIDs []int
	}{
		{
			name: "nil RackConfig returns default rack with ID 0",
			cluster: &ackov1alpha1.AerospikeCluster{
				Spec: ackov1alpha1.AerospikeClusterSpec{
					RackConfig: nil,
				},
			},
			wantLen: 1,
			wantIDs: []int{0},
		},
		{
			name: "empty Racks slice returns default rack with ID 0",
			cluster: &ackov1alpha1.AerospikeCluster{
				Spec: ackov1alpha1.AerospikeClusterSpec{
					RackConfig: &ackov1alpha1.RackConfig{
						Racks: []ackov1alpha1.Rack{},
					},
				},
			},
			wantLen: 1,
			wantIDs: []int{0},
		},
		{
			name: "populated Racks returns racks as-is",
			cluster: &ackov1alpha1.AerospikeCluster{
				Spec: ackov1alpha1.AerospikeClusterSpec{
					RackConfig: &ackov1alpha1.RackConfig{
						Racks: []ackov1alpha1.Rack{
							{ID: 1},
							{ID: 2},
							{ID: 3},
						},
					},
				},
			},
			wantLen: 3,
			wantIDs: []int{1, 2, 3},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := r.getRacks(tc.cluster)
			if len(got) != tc.wantLen {
				t.Fatalf("getRacks() returned %d racks, want %d", len(got), tc.wantLen)
			}
			for i, wantID := range tc.wantIDs {
				if got[i].ID != wantID {
					t.Errorf("getRacks()[%d].ID = %d, want %d", i, got[i].ID, wantID)
				}
			}
		})
	}
}

func TestGetRackSize(t *testing.T) {
	r := &AerospikeClusterReconciler{}

	tests := []struct {
		name      string
		totalSize int32
		racks     []ackov1alpha1.Rack
		rackIndex int
		want      int32
	}{
		{
			name:      "1 rack, 5 pods",
			totalSize: 5,
			racks:     []ackov1alpha1.Rack{{ID: 0}},
			rackIndex: 0,
			want:      5,
		},
		{
			name:      "2 racks even split (6 pods) - rack 0",
			totalSize: 6,
			racks:     []ackov1alpha1.Rack{{ID: 0}, {ID: 1}},
			rackIndex: 0,
			want:      3,
		},
		{
			name:      "2 racks even split (6 pods) - rack 1",
			totalSize: 6,
			racks:     []ackov1alpha1.Rack{{ID: 0}, {ID: 1}},
			rackIndex: 1,
			want:      3,
		},
		{
			name:      "2 racks uneven split (5 pods) - rack 0 gets extra",
			totalSize: 5,
			racks:     []ackov1alpha1.Rack{{ID: 0}, {ID: 1}},
			rackIndex: 0,
			want:      3,
		},
		{
			name:      "2 racks uneven split (5 pods) - rack 1",
			totalSize: 5,
			racks:     []ackov1alpha1.Rack{{ID: 0}, {ID: 1}},
			rackIndex: 1,
			want:      2,
		},
		{
			name:      "3 racks, 7 pods - rack 0 gets extra",
			totalSize: 7,
			racks:     []ackov1alpha1.Rack{{ID: 0}, {ID: 1}, {ID: 2}},
			rackIndex: 0,
			want:      3,
		},
		{
			name:      "3 racks, 7 pods - rack 1",
			totalSize: 7,
			racks:     []ackov1alpha1.Rack{{ID: 0}, {ID: 1}, {ID: 2}},
			rackIndex: 1,
			want:      2,
		},
		{
			name:      "3 racks, 7 pods - rack 2",
			totalSize: 7,
			racks:     []ackov1alpha1.Rack{{ID: 0}, {ID: 1}, {ID: 2}},
			rackIndex: 2,
			want:      2,
		},
		{
			name:      "rack count > pod count (3 racks, 2 pods) - rack 0",
			totalSize: 2,
			racks:     []ackov1alpha1.Rack{{ID: 0}, {ID: 1}, {ID: 2}},
			rackIndex: 0,
			want:      1,
		},
		{
			name:      "rack count > pod count (3 racks, 2 pods) - rack 1",
			totalSize: 2,
			racks:     []ackov1alpha1.Rack{{ID: 0}, {ID: 1}, {ID: 2}},
			rackIndex: 1,
			want:      1,
		},
		{
			name:      "rack count > pod count (3 racks, 2 pods) - rack 2",
			totalSize: 2,
			racks:     []ackov1alpha1.Rack{{ID: 0}, {ID: 1}, {ID: 2}},
			rackIndex: 2,
			want:      0,
		},
		{
			name:      "0 pods - all racks get 0 (rack 0)",
			totalSize: 0,
			racks:     []ackov1alpha1.Rack{{ID: 0}, {ID: 1}},
			rackIndex: 0,
			want:      0,
		},
		{
			name:      "0 pods - all racks get 0 (rack 1)",
			totalSize: 0,
			racks:     []ackov1alpha1.Rack{{ID: 0}, {ID: 1}},
			rackIndex: 1,
			want:      0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cluster := &ackov1alpha1.AerospikeCluster{
				Spec: ackov1alpha1.AerospikeClusterSpec{
					Size: tc.totalSize,
				},
			}
			got := r.getRackSize(cluster, tc.racks, tc.rackIndex)
			if got != tc.want {
				t.Errorf("getRackSize(size=%d, numRacks=%d, rackIndex=%d) = %d, want %d",
					tc.totalSize, len(tc.racks), tc.rackIndex, got, tc.want)
			}
		})
	}
}

func TestGetRackSize_SumInvariant(t *testing.T) {
	r := &AerospikeClusterReconciler{}

	tests := []struct {
		totalSize int32
		numRacks  int
	}{
		{0, 1},
		{1, 1},
		{5, 1},
		{6, 2},
		{5, 2},
		{7, 3},
		{2, 3},
		{8, 3},
		{1, 4},
		{8, 8},
		{3, 5},
	}

	for _, tc := range tests {
		racks := make([]ackov1alpha1.Rack, tc.numRacks)
		for i := range racks {
			racks[i] = ackov1alpha1.Rack{ID: i}
		}
		cluster := &ackov1alpha1.AerospikeCluster{
			Spec: ackov1alpha1.AerospikeClusterSpec{
				Size: tc.totalSize,
			},
		}

		var sum int32
		for i := range racks {
			sum += r.getRackSize(cluster, racks, i)
		}
		if sum != tc.totalSize {
			t.Errorf("sum of getRackSize(totalSize=%d, numRacks=%d) = %d, want %d",
				tc.totalSize, tc.numRacks, sum, tc.totalSize)
		}
	}
}

func TestSetPhasePreservesPendingRestartPods(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := ackov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	stored := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
	}

	reconciler := &AerospikeClusterReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&ackov1alpha1.AerospikeCluster{}).
			WithObjects(stored).
			Build(),
		Scheme: scheme,
	}

	cluster := stored.DeepCopy()
	cluster.Status.PendingRestartPods = []string{"demo-2", "demo-1"}

	if err := reconciler.setPhase(context.Background(), cluster, ackov1alpha1.AerospikePhaseRollingRestart, "Rolling restart in progress"); err != nil {
		t.Fatalf("setPhase() error = %v", err)
	}

	updated := &ackov1alpha1.AerospikeCluster{}
	if err := reconciler.Get(context.Background(), types.NamespacedName{Name: stored.Name, Namespace: stored.Namespace}, updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if updated.Status.Phase != ackov1alpha1.AerospikePhaseRollingRestart {
		t.Fatalf("Phase = %q, want %q", updated.Status.Phase, ackov1alpha1.AerospikePhaseRollingRestart)
	}
	if updated.Status.PhaseReason != "Rolling restart in progress" {
		t.Fatalf("PhaseReason = %q, want %q", updated.Status.PhaseReason, "Rolling restart in progress")
	}
	if len(updated.Status.PendingRestartPods) != 2 || updated.Status.PendingRestartPods[0] != "demo-2" || updated.Status.PendingRestartPods[1] != "demo-1" {
		t.Fatalf("PendingRestartPods = %v, want [demo-2 demo-1]", updated.Status.PendingRestartPods)
	}
}

func TestSetPhaseUpdatesPendingRestartPodsWhenPhaseUnchanged(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := ackov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	stored := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Status: ackov1alpha1.AerospikeClusterStatus{
			Phase:              ackov1alpha1.AerospikePhaseRollingRestart,
			PhaseReason:        "Rolling restart in progress",
			PendingRestartPods: []string{"demo-0"},
		},
	}

	reconciler := &AerospikeClusterReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&ackov1alpha1.AerospikeCluster{}).
			WithObjects(stored).
			Build(),
		Scheme: scheme,
	}

	cluster := stored.DeepCopy()
	cluster.Status.PendingRestartPods = []string{"demo-2", "demo-1"}

	if err := reconciler.setPhase(context.Background(), cluster, ackov1alpha1.AerospikePhaseRollingRestart, "Rolling restart in progress"); err != nil {
		t.Fatalf("setPhase() error = %v", err)
	}

	updated := &ackov1alpha1.AerospikeCluster{}
	if err := reconciler.Get(context.Background(), types.NamespacedName{Name: stored.Name, Namespace: stored.Namespace}, updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if len(updated.Status.PendingRestartPods) != 2 || updated.Status.PendingRestartPods[0] != "demo-2" || updated.Status.PendingRestartPods[1] != "demo-1" {
		t.Fatalf("PendingRestartPods = %v, want [demo-2 demo-1]", updated.Status.PendingRestartPods)
	}
}

func TestTruncateUTF8(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxBytes int
		want     string
	}{
		{
			name:     "short ASCII string unchanged",
			input:    "hello",
			maxBytes: 256,
			want:     "hello",
		},
		{
			name:     "exact limit ASCII unchanged",
			input:    strings.Repeat("a", 256),
			maxBytes: 256,
			want:     strings.Repeat("a", 256),
		},
		{
			name:     "long ASCII truncated within budget",
			input:    strings.Repeat("a", 300),
			maxBytes: 256,
			want:     strings.Repeat("a", 253) + "...",
		},
		{
			name:     "empty string unchanged",
			input:    "",
			maxBytes: 256,
			want:     "",
		},
		{
			name:     "multi-byte at boundary not split",
			input:    strings.Repeat("a", 255) + "한글",
			maxBytes: 256,
			want:     strings.Repeat("a", 253) + "...",
		},
		{
			name:     "multi-byte fully fits",
			input:    strings.Repeat("a", 253) + "한",
			maxBytes: 256,
			want:     strings.Repeat("a", 253) + "한",
		},
		{
			name:     "all multi-byte truncated",
			input:    strings.Repeat("한", 100),
			maxBytes: 10,
			want:     strings.Repeat("한", 2) + "...",
		},
		// maxBytes smaller than the "..." suffix (3 bytes): the suffix must be
		// dropped so the result never exceeds maxBytes.
		{
			name:     "maxBytes 0 returns empty",
			input:    "hello",
			maxBytes: 0,
			want:     "",
		},
		{
			name:     "maxBytes 1 returns single byte without suffix",
			input:    "hello",
			maxBytes: 1,
			want:     "h",
		},
		{
			name:     "maxBytes 2 returns two bytes without suffix",
			input:    "hello",
			maxBytes: 2,
			want:     "he",
		},
		{
			name:     "maxBytes 3 equals suffix length returns three bytes without suffix",
			input:    "hello",
			maxBytes: 3,
			want:     "hel",
		},
		{
			name:     "maxBytes below suffix does not split multi-byte char",
			input:    "한글",
			maxBytes: 2,
			want:     "", // first rune is 3 bytes; cannot fit 2 bytes, so empty
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateUTF8(tc.input, tc.maxBytes)
			if got != tc.want {
				t.Errorf("truncateUTF8(%q, %d) = %q, want %q", tc.input, tc.maxBytes, got, tc.want)
			}
			if len(got) > tc.maxBytes && tc.maxBytes >= 0 {
				t.Errorf("truncateUTF8(%q, %d) returned %d bytes, exceeds maxBytes", tc.input, tc.maxBytes, len(got))
			}
		})
	}
}

func TestPodReadyPredicate_Update(t *testing.T) {
	p := podReadyPredicate{}

	makePod := func(ready corev1.ConditionStatus) *corev1.Pod {
		pod := &corev1.Pod{}
		if ready != "" {
			pod.Status.Conditions = []corev1.PodCondition{
				{Type: corev1.PodReady, Status: ready},
			}
		}
		return pod
	}

	tests := []struct {
		name string
		old  *corev1.Pod
		new  *corev1.Pod
		want bool
	}{
		{
			name: "no change (both true)",
			old:  makePod(corev1.ConditionTrue),
			new:  makePod(corev1.ConditionTrue),
			want: false,
		},
		{
			name: "no change (both false)",
			old:  makePod(corev1.ConditionFalse),
			new:  makePod(corev1.ConditionFalse),
			want: false,
		},
		{
			name: "false to true",
			old:  makePod(corev1.ConditionFalse),
			new:  makePod(corev1.ConditionTrue),
			want: true,
		},
		{
			name: "true to false",
			old:  makePod(corev1.ConditionTrue),
			new:  makePod(corev1.ConditionFalse),
			want: true,
		},
		{
			name: "no condition to true",
			old:  makePod(""),
			new:  makePod(corev1.ConditionTrue),
			want: true,
		},
		{
			name: "no condition on both",
			old:  makePod(""),
			new:  makePod(""),
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := p.Update(event.UpdateEvent{
				ObjectOld: tc.old,
				ObjectNew: tc.new,
			})
			if got != tc.want {
				t.Errorf("podReadyPredicate.Update() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPodReadyPredicate_CreateReturnsFalse(t *testing.T) {
	p := podReadyPredicate{}
	if p.Create(event.CreateEvent{Object: &corev1.Pod{}}) {
		t.Error("podReadyPredicate.Create() = true, want false")
	}
}

func TestPodReadyPredicate_DeleteReturnsTrue(t *testing.T) {
	p := podReadyPredicate{}
	if !p.Delete(event.DeleteEvent{Object: &corev1.Pod{}}) {
		t.Error("podReadyPredicate.Delete() = false, want true")
	}
}

func TestPodReadyPredicate_GenericReturnsFalse(t *testing.T) {
	p := podReadyPredicate{}
	if p.Generic(event.GenericEvent{Object: &corev1.Pod{}}) {
		t.Error("podReadyPredicate.Generic() = true, want false")
	}
}
