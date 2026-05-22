package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/utils"
)

// statusConflictPod builds a cluster pod with the given readiness state.
func statusConflictPod(clusterName, name string, ready bool) *corev1.Pod {
	pod := &corev1.Pod{}
	pod.Name = name
	pod.Namespace = ctrlTestNamespace
	pod.Labels = utils.SelectorLabelsForCluster(clusterName)
	pod.Status.Phase = corev1.PodRunning
	pod.Status.PodIP = "10.0.0.1"
	if ready {
		pod.Status.Conditions = []corev1.PodCondition{
			{Type: corev1.PodReady, Status: corev1.ConditionTrue},
		}
	}
	return pod
}

// TestUpdateStatusAndPhase_ConflictRetryRecomputesPodStatus is the regression
// test for the updateStatusAndPhase conflict-retry fix. On a 409 conflict the
// retry path must re-run populateStatus on the refetched object so it publishes
// *fresh* pod readiness, instead of stamping a pre-conflict computedStatus
// snapshot (which could carry stale pod-readiness) onto the refetched object.
//
// The interceptor returns a conflict on the first Status().Update and, at that
// same moment, flips the pod from NotReady to Ready on the server. With the
// buggy snapshot-stamping code the retry would publish the stale snapshot
// (readyCount captured as 0). With the fix the retry re-lists pods and publishes
// the fresh state (readyCount = 1).
func TestUpdateStatusAndPhase_ConflictRetryRecomputesPodStatus(t *testing.T) {
	scheme := rollingRestartScheme(t)

	const (
		clusterName = "demo"
		podName     = "demo-0"
	)
	namespace := ctrlTestNamespace

	cluster := &ackov1alpha1.AerospikeCluster{}
	cluster.Name = clusterName
	cluster.Namespace = namespace
	cluster.Spec.Size = 1
	cluster.Spec.Image = "aerospike:ce-8.1.1.1"
	cluster.Spec.AerospikeConfig = &ackov1alpha1.AerospikeConfigSpec{
		Value: map[string]any{"service": map[string]any{}},
	}

	// Pod starts NotReady.
	pod := statusConflictPod(clusterName, podName, false)

	key := types.NamespacedName{Name: clusterName, Namespace: namespace}

	var base client.WithWatch
	conflictReturned := false
	base = fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&ackov1alpha1.AerospikeCluster{}).
		WithObjects(cluster, pod).
		Build()

	wrapped := interceptor.NewClient(base, interceptor.Funcs{
		SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			if sub == "status" && !conflictReturned {
				conflictReturned = true
				// Concurrently flip the pod to Ready, then reject the write
				// with a conflict so the retry path runs.
				live := &corev1.Pod{}
				if err := base.Get(ctx, types.NamespacedName{Name: podName, Namespace: namespace}, live); err != nil {
					t.Fatalf("interceptor: Get pod error = %v", err)
				}
				live.Status.Conditions = []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				}
				if err := base.Status().Update(ctx, live); err != nil {
					t.Fatalf("interceptor: pod ready update error = %v", err)
				}
				return apierrors.NewConflict(
					schema.GroupResource{Group: "acko.io", Resource: "aerospikeclusters"},
					clusterName, errTestConflict)
			}
			return c.SubResource(sub).Update(ctx, obj, opts...)
		},
	})

	reconciler := &AerospikeClusterReconciler{
		Client:   wrapped,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(8),
	}

	err := reconciler.updateStatusAndPhase(context.Background(), key,
		ackov1alpha1.AerospikePhaseInProgress, "Reconciling", StatusUpdateOpts{})
	if err != nil {
		t.Fatalf("updateStatusAndPhase() error = %v", err)
	}

	got := &ackov1alpha1.AerospikeCluster{}
	if err := base.Get(context.Background(), key, got); err != nil {
		t.Fatalf("Get final cluster error = %v", err)
	}

	// The retry must have re-run populateStatus against the refetched object,
	// observing the now-Ready pod. A stale snapshot would have published Size=0.
	if got.Status.Size != 1 {
		t.Errorf("Status.Size = %d, want 1 — conflict-retry published stale pod readiness", got.Status.Size)
	}
	if ps, ok := got.Status.Pods[podName]; !ok || !ps.IsRunningAndReady {
		t.Errorf("pod %s IsRunningAndReady = %v (present=%v), want true — conflict-retry did not recompute pod status",
			podName, ps.IsRunningAndReady, ok)
	}
}

// errTestConflict is a sentinel cause for the synthetic conflict error.
var errTestConflict = &testConflictCause{}

type testConflictCause struct{}

func (e *testConflictCause) Error() string { return "object was modified" }
