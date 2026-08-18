package controller

import (
	"context"
	"fmt"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/utils"
)

// --- removing a rack drains it instead of deleting it outright ---
//
// cleanupRemovedRacks deleted a removed rack's StatefulSet with foreground
// propagation and nothing else: no migration check, no quiesce, no batching —
// while the scale-down path in the same file has all three. Dropping a rack from
// spec.rackConfig.racks therefore terminated every Aerospike node in that rack at
// once, possibly mid-migration, and if the rack held the surviving copies of a
// partition those records were gone (#342).
//
// The rack is now drained one scale-down batch at a time, gated on migration and
// quiescing the pods on their way out, and the StatefulSet is deleted only once
// it is at zero replicas.
//
// TestCleanupRemovedRacks_DoesNotDeleteWhenMigrationCheckFails is the gate
// regression test: pre-fix the StatefulSet was deleted regardless.

const (
	teardownCluster  = "demo"
	teardownKeptRack = 1
	teardownGoneRack = 2
	teardownSecret   = "missing-admin-secret"
)

func teardownScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := ackov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("acko AddToScheme() error = %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("corev1 AddToScheme() error = %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("appsv1 AddToScheme() error = %v", err)
	}
	return scheme
}

// teardownClusterSpec builds a cluster whose rackConfig still lists the kept rack
// only, so the gone rack's StatefulSet is a removed rack. ACL is enabled with a
// Secret that does not exist, which makes the migration check fail
// deterministically and without touching the network.
func teardownClusterSpec(withACL bool, batchSize *intstr.IntOrString) *ackov1alpha1.AerospikeCluster {
	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      teardownCluster,
			Namespace: ctrlTestNamespace,
			UID:       "cluster-uid-teardown",
		},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size:  3,
			Image: "aerospike:ce-8.1.1.1",
			RackConfig: &ackov1alpha1.RackConfig{
				Racks:              []ackov1alpha1.Rack{{ID: teardownKeptRack}},
				ScaleDownBatchSize: batchSize,
			},
		},
	}
	if withACL {
		cluster.Spec.AerospikeAccessControl = &ackov1alpha1.AerospikeAccessControlSpec{
			Users: []ackov1alpha1.AerospikeUserSpec{{
				Name:       "admin",
				SecretName: teardownSecret,
				Roles:      []string{"sys-admin", "user-admin"},
			}},
		}
	}
	return cluster
}

func teardownSts(clusterName string, rackID int, replicas int32) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.StatefulSetName(clusterName, rackID),
			Namespace: ctrlTestNamespace,
			Labels:    utils.LabelsForCluster(clusterName),
		},
		Spec: appsv1.StatefulSetSpec{Replicas: &replicas},
	}
}

// teardownPod builds a ready pod for the rack, so quiesceNodeBeforeDeletion does
// not short-circuit on readiness and checkScaleDownReadiness can count it.
func teardownPod(clusterName string, rackID, ordinal int) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%d", utils.StatefulSetName(clusterName, rackID), ordinal),
			Namespace: ctrlTestNamespace,
			Labels:    utils.LabelsForRack(clusterName, rackID),
		},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}
}

func teardownReconciler(t *testing.T, objs ...client.Object) (*AerospikeClusterReconciler, *record.FakeRecorder) {
	t.Helper()
	scheme := teardownScheme(t)
	recorder := record.NewFakeRecorder(32)
	return &AerospikeClusterReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&ackov1alpha1.AerospikeCluster{}).
			WithObjects(objs...).Build(),
		Scheme:   scheme,
		Recorder: recorder,
	}, recorder
}

func stsExists(t *testing.T, c client.Client, name string) bool {
	t.Helper()
	err := c.Get(context.Background(),
		types.NamespacedName{Name: name, Namespace: ctrlTestNamespace}, &appsv1.StatefulSet{})
	switch {
	case err == nil:
		return true
	case apierrors.IsNotFound(err):
		return false
	default:
		t.Fatalf("Get StatefulSet %s: %v", name, err)
		return false
	}
}

func stsReplicas(t *testing.T, c client.Client, name string) int32 {
	t.Helper()
	sts := &appsv1.StatefulSet{}
	if err := c.Get(context.Background(),
		types.NamespacedName{Name: name, Namespace: ctrlTestNamespace}, sts); err != nil {
		t.Fatalf("Get StatefulSet %s: %v", name, err)
	}
	return statefulSetReplicas(sts)
}

// TestCleanupRemovedRacks_DoesNotDeleteWhenMigrationCheckFails is the core
// data-safety regression. Pre-fix the StatefulSet was deleted unconditionally.
func TestCleanupRemovedRacks_DoesNotDeleteWhenMigrationCheckFails(t *testing.T) {
	cluster := teardownClusterSpec(true, nil)
	goneName := utils.StatefulSetName(teardownCluster, teardownGoneRack)

	r, recorder := teardownReconciler(t, cluster,
		teardownSts(teardownCluster, teardownKeptRack, 3),
		teardownSts(teardownCluster, teardownGoneRack, 3),
		teardownPod(teardownCluster, teardownGoneRack, 0),
		teardownPod(teardownCluster, teardownGoneRack, 1),
		teardownPod(teardownCluster, teardownGoneRack, 2),
	)

	// Sanity: the migration check really does fail here.
	if _, err := r.isMigrationInProgress(context.Background(), cluster); err == nil {
		t.Fatal("isMigrationInProgress() returned no error; this test needs the check to fail")
	}

	deferred, err := r.cleanupRemovedRacks(context.Background(), cluster, cluster.Spec.RackConfig.Racks)
	if err != nil {
		t.Fatalf("cleanupRemovedRacks() error = %v", err)
	}
	if !deferred {
		t.Error("cleanupRemovedRacks() deferred = false, want true so the caller requeues")
	}

	if !stsExists(t, r.Client, goneName) {
		t.Fatal("removed rack's StatefulSet was deleted while the migration check was failing; " +
			"that terminates every node in the rack with partitions possibly still moving")
	}
	if got := stsReplicas(t, r.Client, goneName); got != 3 {
		t.Errorf("removed rack replicas = %d, want 3 (untouched while the gate is closed)", got)
	}
	if events := drainRecorderEvents(recorder); !containsEvent(events, EventScaleDownDeferred) {
		t.Errorf("expected a %s event, got %v", EventScaleDownDeferred, events)
	}
}

// TestCleanupRemovedRacks_KeptRackUntouched pins that the gate does not make the
// teardown path start touching racks that are still in the spec.
func TestCleanupRemovedRacks_KeptRackUntouched(t *testing.T) {
	cluster := teardownClusterSpec(true, nil)
	keptName := utils.StatefulSetName(teardownCluster, teardownKeptRack)

	r, _ := teardownReconciler(t, cluster,
		teardownSts(teardownCluster, teardownKeptRack, 3),
		teardownSts(teardownCluster, teardownGoneRack, 3),
	)

	if _, err := r.cleanupRemovedRacks(context.Background(), cluster, cluster.Spec.RackConfig.Racks); err != nil {
		t.Fatalf("cleanupRemovedRacks() error = %v", err)
	}
	if !stsExists(t, r.Client, keptName) {
		t.Fatal("StatefulSet for a rack still in the spec was deleted")
	}
	if got := stsReplicas(t, r.Client, keptName); got != 3 {
		t.Errorf("kept rack replicas = %d, want 3", got)
	}
}

// TestDrainRemovedRack_ScalesInBatchesAndQuiesces covers the post-gate half:
// batching, quiesce of the pods going away, and never deleting on the pass that
// scales down.
func TestDrainRemovedRack_ScalesInBatchesAndQuiesces(t *testing.T) {
	batchOfOne := intstr.FromInt32(1)
	tests := []struct {
		name            string
		batchSize       *intstr.IntOrString
		replicas        int32
		wantTarget      int32
		wantDrained     bool
		wantQuiescePods []int // ordinals expected to be quiesced
	}{
		{
			// Default batch size is the whole delta, so a removed rack goes
			// straight to zero — but still quiesced, and still not deleted on
			// this pass.
			name:            "default batch drains the whole rack in one step",
			replicas:        3,
			wantTarget:      0,
			wantDrained:     false,
			wantQuiescePods: []int{0, 1, 2},
		},
		{
			name:            "scaleDownBatchSize 1 removes one pod per pass",
			batchSize:       &batchOfOne,
			replicas:        3,
			wantTarget:      2,
			wantDrained:     false,
			wantQuiescePods: []int{2},
		},
		{
			name:        "a rack already at zero is reported drained",
			replicas:    0,
			wantTarget:  0,
			wantDrained: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := teardownClusterSpec(false, tt.batchSize)
			sts := teardownSts(teardownCluster, teardownGoneRack, tt.replicas)

			objs := []client.Object{cluster, sts}
			for i := range int(tt.replicas) {
				objs = append(objs, teardownPod(teardownCluster, teardownGoneRack, i))
			}
			r, recorder := teardownReconciler(t, objs...)

			// Re-read so the object carries the resourceVersion the fake client
			// assigned; drainRemovedRack patches it.
			live := &appsv1.StatefulSet{}
			if err := r.Get(context.Background(),
				types.NamespacedName{Name: sts.Name, Namespace: ctrlTestNamespace}, live); err != nil {
				t.Fatalf("Get StatefulSet: %v", err)
			}

			drained, err := r.drainRemovedRack(context.Background(), cluster, live, teardownGoneRack)
			if err != nil {
				t.Fatalf("drainRemovedRack() error = %v", err)
			}
			if drained != tt.wantDrained {
				t.Fatalf("drainRemovedRack() drained = %v, want %v", drained, tt.wantDrained)
			}

			if got := stsReplicas(t, r.Client, sts.Name); got != tt.wantTarget {
				t.Errorf("replicas after drain = %d, want %d", got, tt.wantTarget)
			}
			if !stsExists(t, r.Client, sts.Name) {
				t.Error("drainRemovedRack deleted the StatefulSet; deleting is the caller's job, " +
					"and only once the rack is at zero replicas")
			}

			// A single event must mention BOTH the quiesce reason and the pod;
			// matching them independently across the list would pass even if the
			// wrong pod were quiesced.
			events := drainRecorderEvents(recorder)
			quiesced := map[string]bool{}
			for _, e := range events {
				if !strings.Contains(e, EventNodeQuiesceStarted) {
					continue
				}
				for ordinal := range int(tt.replicas) {
					podName := fmt.Sprintf("%s-%d", sts.Name, ordinal)
					if strings.Contains(e, podName) {
						quiesced[podName] = true
					}
				}
			}
			for _, ordinal := range tt.wantQuiescePods {
				podName := fmt.Sprintf("%s-%d", sts.Name, ordinal)
				if !quiesced[podName] {
					t.Errorf("expected a quiesce attempt for %s, events = %v", podName, events)
				}
			}
			// Pods that survive this batch must not be quiesced.
			for ordinal := range int(tt.wantTarget) {
				podName := fmt.Sprintf("%s-%d", sts.Name, ordinal)
				if quiesced[podName] {
					t.Errorf("pod %s survives this batch but was quiesced; events = %v", podName, events)
				}
			}
		})
	}
}

// TestCleanupRemovedRacks_DeletesOnceDrained pins the far end: with the rack at
// zero replicas and no pods left, the StatefulSet is deleted and the ConfigMap
// cleaned up, and the pass no longer defers.
func TestCleanupRemovedRacks_DeletesOnceDrained(t *testing.T) {
	cluster := teardownClusterSpec(false, nil)
	goneName := utils.StatefulSetName(teardownCluster, teardownGoneRack)
	cmName := utils.ConfigMapName(teardownCluster, teardownGoneRack)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: ctrlTestNamespace},
	}
	r, _ := teardownReconciler(t, cluster,
		teardownSts(teardownCluster, teardownKeptRack, 3),
		teardownSts(teardownCluster, teardownGoneRack, 0),
		configMap,
	)

	deferred, err := r.cleanupRemovedRacks(context.Background(), cluster, cluster.Spec.RackConfig.Racks)
	if err != nil {
		t.Fatalf("cleanupRemovedRacks() error = %v", err)
	}
	if deferred {
		t.Error("cleanupRemovedRacks() deferred = true on a fully drained rack, want false")
	}
	if stsExists(t, r.Client, goneName) {
		t.Error("drained rack's StatefulSet was not deleted")
	}

	err = r.Get(context.Background(), types.NamespacedName{Name: cmName, Namespace: ctrlTestNamespace},
		&corev1.ConfigMap{})
	if !apierrors.IsNotFound(err) {
		t.Errorf("removed rack's ConfigMap still present, Get error = %v", err)
	}
}

// TestCleanupRemovedRacks_NoMigrationCheckWhenNothingToDrain pins that a pass
// with only already-drained racks does not open an Aerospike client at all. The
// cluster here has ACL configured with a missing Secret, so a migration check
// would fail and defer; the rack must still be cleaned up.
func TestCleanupRemovedRacks_NoMigrationCheckWhenNothingToDrain(t *testing.T) {
	cluster := teardownClusterSpec(true, nil)
	goneName := utils.StatefulSetName(teardownCluster, teardownGoneRack)

	r, _ := teardownReconciler(t, cluster,
		teardownSts(teardownCluster, teardownKeptRack, 3),
		teardownSts(teardownCluster, teardownGoneRack, 0),
	)

	deferred, err := r.cleanupRemovedRacks(context.Background(), cluster, cluster.Spec.RackConfig.Racks)
	if err != nil {
		t.Fatalf("cleanupRemovedRacks() error = %v", err)
	}
	if deferred {
		t.Error("cleanupRemovedRacks() deferred = true; a rack with nothing to drain must not be gated on migration")
	}
	if stsExists(t, r.Client, goneName) {
		t.Error("drained rack's StatefulSet was not deleted")
	}
}
