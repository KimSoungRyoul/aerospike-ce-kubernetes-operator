package controller

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/utils"
)

// --- status.replicas backs the scale subresource ---
//
// The scale subresource used to point statuspath at .status.size, which
// populateStatus sets to the READY pod count. Every consumer of the subresource
// contract — HPA, KEDA, `kubectl scale --current-replicas` — reads that field as
// "current replicas", so during any rolling restart they saw an undercount and
// an HPA could scale a healthy cluster down.
//
// status.replicas now carries the selector-matched pod count and the marker
// points at it; status.size keeps the readiness view for the printer columns.
//
// TestPopulateStatus_ReplicasCountsAllSelectorMatchedPods is the regression
// test: its "one of three ready" case asserts replicas=3 while size=1, and that
// case fails on the pre-fix code, where the field did not exist and the
// subresource read size=1.

const (
	replicasTestCluster = "demo"
	replicasTestForeign = "other-workload"
)

// replicasTestPod builds a pod carrying the cluster's full label set, so it is
// matched by the same selector populateStatus lists on.
func replicasTestPod(name string, ready bool) *corev1.Pod {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ctrlTestNamespace,
			Labels:    utils.LabelsForCluster(replicasTestCluster),
		},
		Status: corev1.PodStatus{Phase: corev1.PodPending},
	}
	if ready {
		pod.Status = corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		}
	}
	return pod
}

// replicasTestForeignPod is a ready pod in the same namespace that belongs to a
// different workload. It must not be counted by either field.
func replicasTestForeignPod(name string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ctrlTestNamespace,
			Labels:    utils.LabelsForCluster(replicasTestForeign),
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}
}

func replicasTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := ackov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("acko AddToScheme() error = %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("corev1 AddToScheme() error = %v", err)
	}
	return scheme
}

func TestPopulateStatus_ReplicasCountsAllSelectorMatchedPods(t *testing.T) {
	tests := []struct {
		name         string
		specSize     int32
		readyPods    int
		notReadyPods int
		foreignPods  int
		wantReplicas int32
		wantSize     int32
		wantHealth   string
	}{
		{
			name:         "converged cluster: replicas equals ready count",
			specSize:     3,
			readyPods:    3,
			wantReplicas: 3,
			wantSize:     3,
			wantHealth:   "3/3",
		},
		{
			// The regression case. Mid rolling restart two pods are not ready,
			// but three replicas exist. An HPA reading the old statuspath saw 1.
			name:         "one of three ready: replicas stays at the replica count",
			specSize:     3,
			readyPods:    1,
			notReadyPods: 2,
			wantReplicas: 3,
			wantSize:     1,
			wantHealth:   "1/3",
		},
		{
			name:         "no pod ready: replicas still reports the pods that exist",
			specSize:     2,
			notReadyPods: 2,
			wantReplicas: 2,
			wantSize:     0,
			wantHealth:   "0/2",
		},
		{
			name:         "no pods at all",
			specSize:     1,
			wantReplicas: 0,
			wantSize:     0,
			wantHealth:   "0/1",
		},
		{
			// status.selector is published alongside status.replicas and an HPA
			// uses it to find the pods. Both must agree on which pods count.
			name:         "pods of another workload in the namespace are not counted",
			specSize:     1,
			readyPods:    1,
			foreignPods:  2,
			wantReplicas: 1,
			wantSize:     1,
			wantHealth:   "1/1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := replicasTestScheme(t)

			objs := []client.Object{}
			for i := range tt.readyPods {
				objs = append(objs, replicasTestPod(fmt.Sprintf("%s-0-%d", replicasTestCluster, i), true))
			}
			for i := range tt.notReadyPods {
				objs = append(objs,
					replicasTestPod(fmt.Sprintf("%s-0-%d", replicasTestCluster, tt.readyPods+i), false))
			}
			for i := range tt.foreignPods {
				objs = append(objs, replicasTestForeignPod(fmt.Sprintf("%s-%d", replicasTestForeign, i)))
			}

			cluster := &ackov1alpha1.AerospikeCluster{
				ObjectMeta: metav1.ObjectMeta{Name: replicasTestCluster, Namespace: ctrlTestNamespace},
				Spec: ackov1alpha1.AerospikeClusterSpec{
					Size:  tt.specSize,
					Image: "aerospike:ce-8.1.1.1",
				},
			}

			r := &AerospikeClusterReconciler{
				Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(),
				Scheme:   scheme,
				Recorder: record.NewFakeRecorder(8),
			}

			readyCount, err := r.populateStatus(context.Background(), cluster)
			if err != nil {
				t.Fatalf("populateStatus() error = %v", err)
			}

			if cluster.Status.Replicas != tt.wantReplicas {
				t.Errorf("Status.Replicas = %d, want %d", cluster.Status.Replicas, tt.wantReplicas)
			}
			if cluster.Status.Size != tt.wantSize {
				t.Errorf("Status.Size = %d, want %d", cluster.Status.Size, tt.wantSize)
			}
			if readyCount != tt.wantSize {
				t.Errorf("populateStatus() readyCount = %d, want %d", readyCount, tt.wantSize)
			}
			if cluster.Status.Health != tt.wantHealth {
				t.Errorf("Status.Health = %q, want %q", cluster.Status.Health, tt.wantHealth)
			}
			// The published selector has to match the pods that were counted,
			// or an HPA reading replicas will divide by a different pod set.
			wantSelector := buildSelectorString(utils.SelectorLabelsForCluster(replicasTestCluster))
			if cluster.Status.Selector != wantSelector {
				t.Errorf("Status.Selector = %q, want %q", cluster.Status.Selector, wantSelector)
			}
		})
	}
}

// TestStatusUnchanged_DetectsReplicasChange pins that a replica-count change on
// its own forces a status write. Without it, a stale replica count could stay
// published to an HPA because every other compared field happened to match.
func TestStatusUnchanged_DetectsReplicasChange(t *testing.T) {
	const phase = ackov1alpha1.AerospikePhaseCompleted

	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: replicasTestCluster, Namespace: ctrlTestNamespace, Generation: 1},
		Status: ackov1alpha1.AerospikeClusterStatus{
			Health:             "1/1",
			Selector:           "app=x",
			Replicas:           2,
			ObservedGeneration: 1,
		},
	}
	prev := statusSnapshot{
		Phase:      phase,
		Size:       1,
		Replicas:   2,
		Health:     "1/1",
		Generation: 1,
		Selector:   "app=x",
	}

	if !statusUnchanged(prev, cluster, 1, phase, "") {
		t.Fatalf("statusUnchanged() = false, want true when nothing changed")
	}

	cluster.Status.Replicas = 3
	if statusUnchanged(prev, cluster, 1, phase, "") {
		t.Fatalf("statusUnchanged() = true, want false when replicas changed")
	}
}
