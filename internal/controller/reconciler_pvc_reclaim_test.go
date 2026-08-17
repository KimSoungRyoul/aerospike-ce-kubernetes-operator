package controller

import (
	"context"
	"fmt"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/utils"
)

// --- PVC reclamation after scale-down ---
//
// The cleanup used to sit inside reconcileStatefulSet's `needsUpdate` branch, so
// it ran only on the pass that performed the scale-down — the one pass on which
// it can never succeed, because the removed pods are still terminating. On the
// next pass the StatefulSet already read the desired size, both hashes matched,
// and the function returned before the cleanup was reachable. With the default
// scale-down batch size returning the whole delta, that deferred pass was the
// only pass, so every cascadeDelete PVC leaked permanently — and because PVC
// names are ordinal-derived, a later scale-up remounted the exact device the
// removed node wrote.
//
// These tests pin both halves: reclamation now happens on the converged pass,
// and it can never select a PVC that a live pod is using.

const pvcReclaimRackID = 0

func pvcReclaimCluster(cascadeDelete bool) *ackov1alpha1.AerospikeCluster {
	return &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: ctrlTestNamespace},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size:    1,
			Image:   "aerospike:ce-8.1.1.1",
			Storage: pvcReclaimStorage(cascadeDelete),
		},
	}
}

func pvcReclaimStorage(cascadeDelete bool) *ackov1alpha1.AerospikeStorageSpec {
	return &ackov1alpha1.AerospikeStorageSpec{
		Volumes: []ackov1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: ackov1alpha1.VolumeSource{
					PersistentVolume: &ackov1alpha1.PersistentVolumeSpec{Size: "10Gi"},
				},
				CascadeDelete: &cascadeDelete,
			},
		},
	}
}

// pvcReclaimPVC builds a PVC named the way a StatefulSet names it —
// <volume>-<stsName>-<ordinal> — carrying the cluster labels the operator
// stamps onto VolumeClaimTemplates, so the label-scoped query matches it.
func pvcReclaimPVC(clusterName string, ordinal int) *corev1.PersistentVolumeClaim {
	stsName := utils.StatefulSetName(clusterName, pvcReclaimRackID)
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("data-%s-%d", stsName, ordinal),
			Namespace: ctrlTestNamespace,
			Labels:    utils.LabelsForCluster(clusterName),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
	}
}

// pvcReclaimPod builds a rack pod named <stsName>-<ordinal>.
func pvcReclaimPod(clusterName string, ordinal int) *corev1.Pod {
	stsName := utils.StatefulSetName(clusterName, pvcReclaimRackID)
	return pvcReclaimNamedPod(clusterName, fmt.Sprintf("%s-%d", stsName, ordinal))
}

func pvcReclaimNamedPod(clusterName, podName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: ctrlTestNamespace,
			Labels:    utils.LabelsForRack(clusterName, pvcReclaimRackID),
		},
	}
}

// pvcNames returns the sorted set of PVC names still present in the namespace.
func pvcNames(t *testing.T, c client.Client, clusterName string) map[string]bool {
	t.Helper()
	present := map[string]bool{}
	for ordinal := 0; ordinal < 6; ordinal++ {
		pvc := pvcReclaimPVC(clusterName, ordinal)
		err := c.Get(context.Background(),
			types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace},
			&corev1.PersistentVolumeClaim{})
		switch {
		case err == nil:
			present[pvc.Name] = true
		case apierrors.IsNotFound(err):
		default:
			t.Fatalf("Get PVC %s: %v", pvc.Name, err)
		}
	}
	return present
}

func pvcReclaimReconciler(scheme *runtime.Scheme, objs ...client.Object) *AerospikeClusterReconciler {
	return &AerospikeClusterReconciler{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(8),
	}
}

// TestReconcileStatefulSet_ReclaimsOrphanedPVCsOnConvergedPass is the
// regression test for the leak. The StatefulSet has already been scaled down to
// 1 replica, the removed pod is gone, and both hashes match — so
// reconcileStatefulSet takes the `!needsUpdate` early return, which is exactly
// where the old code stopped being able to reclaim anything. The orphaned PVC
// must be gone by the end of this pass, and the in-use one must not be.
func TestReconcileStatefulSet_ReclaimsOrphanedPVCsOnConvergedPass(t *testing.T) {
	scheme := rollingRestartScheme(t)

	cluster := pvcReclaimCluster(true)
	rack := &ackov1alpha1.Rack{ID: pvcReclaimRackID}
	stsName := utils.StatefulSetName(cluster.Name, rack.ID)

	const configHash = "cfg-SAME"
	podSpecHash := computePodSpecHash(cluster, rack)
	oneReplica := int32(1)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: stsName, Namespace: ctrlTestNamespace},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &oneReplica,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						utils.ConfigHashAnnotation:  configHash,
						utils.PodSpecHashAnnotation: podSpecHash,
					},
				},
			},
		},
	}

	r := pvcReclaimReconciler(scheme,
		sts,
		pvcReclaimPod(cluster.Name, 0),
		pvcReclaimPVC(cluster.Name, 0), // in use by demo-0-0
		pvcReclaimPVC(cluster.Name, 1), // orphaned by an earlier scale-down
	)

	deferred, err := r.reconcileStatefulSet(
		context.Background(), cluster, rack, nil, configHash, 1)
	if err != nil {
		t.Fatalf("reconcileStatefulSet() error = %v", err)
	}
	if deferred {
		t.Fatal("reconcileStatefulSet() deferred = true, want false on a converged rack")
	}

	present := pvcNames(t, r.Client, cluster.Name)
	if !present[fmt.Sprintf("data-%s-0", stsName)] {
		t.Error("PVC at ordinal 0 was deleted; it is in use by a running pod")
	}
	if present[fmt.Sprintf("data-%s-1", stsName)] {
		t.Error("orphaned PVC at ordinal 1 was not reclaimed on the converged pass")
	}
}

// TestReclaimOrphanedRackPVCs_NeverSelectsInUsePVC is the safety test. Deleting
// a PVC that a running Aerospike node has mounted destroys that node's data, so
// only ordinals at or above the StatefulSet's own replica count may ever be
// considered — and never the desired rack size, which during a batched
// scale-down is lower than the count the StatefulSet is actually running.
func TestReclaimOrphanedRackPVCs_NeverSelectsInUsePVC(t *testing.T) {
	tests := []struct {
		name string
		// specReplicas is the StatefulSet's own spec.replicas.
		specReplicas int32
		// podOrdinals are the pods currently observed for the rack.
		podOrdinals []int
		// pvcOrdinals are the PVCs that exist.
		pvcOrdinals []int
		// wantDeleted lists the ordinals that must be reclaimed; every other
		// PVC in pvcOrdinals must survive.
		wantDeleted []int
	}{
		{
			name:         "fully converged rack reclaims nothing",
			specReplicas: 3,
			podOrdinals:  []int{0, 1, 2},
			pvcOrdinals:  []int{0, 1, 2},
			wantDeleted:  nil,
		},
		{
			name:         "one ordinal above the replica count is reclaimed",
			specReplicas: 2,
			podOrdinals:  []int{0, 1},
			pvcOrdinals:  []int{0, 1, 2},
			wantDeleted:  []int{2},
		},
		{
			name:         "several ordinals above the replica count are reclaimed",
			specReplicas: 1,
			podOrdinals:  []int{0},
			pvcOrdinals:  []int{0, 1, 2, 3},
			wantDeleted:  []int{1, 2, 3},
		},
		{
			// A cluster that is entirely down still keeps every PVC below the
			// replica count: those volumes belong to nodes that will come back.
			name:         "no pods observed still preserves ordinals below the replica count",
			specReplicas: 3,
			podOrdinals:  nil,
			pvcOrdinals:  []int{0, 1, 2, 3},
			wantDeleted:  []int{3},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scheme := rollingRestartScheme(t)
			cluster := pvcReclaimCluster(true)
			stsName := utils.StatefulSetName(cluster.Name, pvcReclaimRackID)

			var objs []client.Object
			for _, ordinal := range tc.podOrdinals {
				objs = append(objs, pvcReclaimPod(cluster.Name, ordinal))
			}
			for _, ordinal := range tc.pvcOrdinals {
				objs = append(objs, pvcReclaimPVC(cluster.Name, ordinal))
			}

			r := pvcReclaimReconciler(scheme, objs...)
			replicas := tc.specReplicas
			r.reclaimOrphanedRackPVCs(context.Background(), cluster,
				pvcReclaimRackID, stsName, &replicas, cluster.Spec.Storage)

			shouldBeGone := map[int]bool{}
			for _, ordinal := range tc.wantDeleted {
				shouldBeGone[ordinal] = true
			}

			present := pvcNames(t, r.Client, cluster.Name)
			for _, ordinal := range tc.pvcOrdinals {
				name := fmt.Sprintf("data-%s-%d", stsName, ordinal)
				if shouldBeGone[ordinal] && present[name] {
					t.Errorf("PVC %s should have been reclaimed (replicas=%d)", name, tc.specReplicas)
				}
				if !shouldBeGone[ordinal] && !present[name] {
					t.Errorf("PVC %s was deleted but its ordinal is below replicas=%d", name, tc.specReplicas)
				}
			}
		})
	}
}

// TestReclaimOrphanedRackPVCs_DefersWhilePodAboveReplicaCountExists covers the
// mid-scale-down window. The pod being removed is still terminating and still
// has its volume mounted, so nothing may be reclaimed until it is gone.
func TestReclaimOrphanedRackPVCs_DefersWhilePodAboveReplicaCountExists(t *testing.T) {
	scheme := rollingRestartScheme(t)
	cluster := pvcReclaimCluster(true)
	stsName := utils.StatefulSetName(cluster.Name, pvcReclaimRackID)

	r := pvcReclaimReconciler(scheme,
		pvcReclaimPod(cluster.Name, 0),
		pvcReclaimPod(cluster.Name, 1), // still terminating
		pvcReclaimPVC(cluster.Name, 0),
		pvcReclaimPVC(cluster.Name, 1),
	)

	replicas := int32(1)
	r.reclaimOrphanedRackPVCs(context.Background(), cluster,
		pvcReclaimRackID, stsName, &replicas, cluster.Spec.Storage)

	present := pvcNames(t, r.Client, cluster.Name)
	for ordinal := 0; ordinal < 2; ordinal++ {
		name := fmt.Sprintf("data-%s-%d", stsName, ordinal)
		if !present[name] {
			t.Errorf("PVC %s was deleted while pod %s-1 is still terminating", name, stsName)
		}
	}
}

// TestReclaimOrphanedRackPVCs_DefersOnUnparseablePodName fails closed: a pod
// whose ordinal cannot be read must never be assumed to sit below the replica
// count. podOrdinal returns 0 for such a name, which would have read as "not an
// orphan candidate" and allowed the deletion to proceed.
func TestReclaimOrphanedRackPVCs_DefersOnUnparseablePodName(t *testing.T) {
	scheme := rollingRestartScheme(t)
	cluster := pvcReclaimCluster(true)
	stsName := utils.StatefulSetName(cluster.Name, pvcReclaimRackID)

	r := pvcReclaimReconciler(scheme,
		pvcReclaimNamedPod(cluster.Name, stsName+"-not-a-number"),
		pvcReclaimPVC(cluster.Name, 0),
		pvcReclaimPVC(cluster.Name, 1),
	)

	replicas := int32(1)
	r.reclaimOrphanedRackPVCs(context.Background(), cluster,
		pvcReclaimRackID, stsName, &replicas, cluster.Spec.Storage)

	present := pvcNames(t, r.Client, cluster.Name)
	if !present[fmt.Sprintf("data-%s-1", stsName)] {
		t.Error("PVC was reclaimed despite a rack pod with an unparseable ordinal")
	}
}

// TestReclaimOrphanedRackPVCs_SkipsWhenReplicaCountUnknown guards the nil case.
// Treating a nil spec.replicas as 0 would make every PVC in the rack an orphan
// candidate — including every in-use one.
func TestReclaimOrphanedRackPVCs_SkipsWhenReplicaCountUnknown(t *testing.T) {
	scheme := rollingRestartScheme(t)
	cluster := pvcReclaimCluster(true)
	stsName := utils.StatefulSetName(cluster.Name, pvcReclaimRackID)

	r := pvcReclaimReconciler(scheme,
		pvcReclaimPVC(cluster.Name, 0),
		pvcReclaimPVC(cluster.Name, 1),
	)

	r.reclaimOrphanedRackPVCs(context.Background(), cluster,
		pvcReclaimRackID, stsName, nil, cluster.Spec.Storage)

	present := pvcNames(t, r.Client, cluster.Name)
	for ordinal := 0; ordinal < 2; ordinal++ {
		name := fmt.Sprintf("data-%s-%d", stsName, ordinal)
		if !present[name] {
			t.Errorf("PVC %s was deleted with an unknown replica count", name)
		}
	}
}

// TestReclaimOrphanedRackPVCs_SkipsAtZeroReplicas keeps whole-rack teardown out
// of this path: at zero replicas every ordinal is >= the replica count, and
// deleting by ordinal would duplicate the rack-removal decision without its
// guards.
func TestReclaimOrphanedRackPVCs_SkipsAtZeroReplicas(t *testing.T) {
	scheme := rollingRestartScheme(t)
	cluster := pvcReclaimCluster(true)
	stsName := utils.StatefulSetName(cluster.Name, pvcReclaimRackID)

	r := pvcReclaimReconciler(scheme,
		pvcReclaimPVC(cluster.Name, 0),
		pvcReclaimPVC(cluster.Name, 1),
	)

	replicas := int32(0)
	r.reclaimOrphanedRackPVCs(context.Background(), cluster,
		pvcReclaimRackID, stsName, &replicas, cluster.Spec.Storage)

	present := pvcNames(t, r.Client, cluster.Name)
	for ordinal := 0; ordinal < 2; ordinal++ {
		name := fmt.Sprintf("data-%s-%d", stsName, ordinal)
		if !present[name] {
			t.Errorf("PVC %s was deleted for a rack at zero replicas", name)
		}
	}
}

// TestReclaimOrphanedRackPVCs_NonCascadeVolumesIssueNoList pins the
// steady-state cost. cascadeDelete defaults to false, so most clusters must not
// pay for a pod List or a PVC List on every reconcile — the check happens before
// either one.
func TestReclaimOrphanedRackPVCs_NonCascadeVolumesIssueNoList(t *testing.T) {
	scheme := rollingRestartScheme(t)
	cluster := pvcReclaimCluster(false)
	stsName := utils.StatefulSetName(cluster.Name, pvcReclaimRackID)

	base := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(pvcReclaimPVC(cluster.Name, 0), pvcReclaimPVC(cluster.Name, 1)).
		Build()

	lists := 0
	wrapped := interceptor.NewClient(base, interceptor.Funcs{
		List: func(ctx context.Context, c client.WithWatch,
			list client.ObjectList, opts ...client.ListOption) error {
			lists++
			return c.List(ctx, list, opts...)
		},
	})

	r := &AerospikeClusterReconciler{
		Client:   wrapped,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(8),
	}

	replicas := int32(1)
	r.reclaimOrphanedRackPVCs(context.Background(), cluster,
		pvcReclaimRackID, stsName, &replicas, cluster.Spec.Storage)

	if lists != 0 {
		t.Errorf("issued %d List call(s) for a cluster with no cascadeDelete volume, want 0", lists)
	}

	present := pvcNames(t, r.Client, cluster.Name)
	for ordinal := 0; ordinal < 2; ordinal++ {
		name := fmt.Sprintf("data-%s-%d", stsName, ordinal)
		if !present[name] {
			t.Errorf("non-cascadeDelete PVC %s was deleted", name)
		}
	}
}

// TestReclaimOrphanedRackPVCs_Idempotent pins that repeated passes are safe —
// the function now runs on every reconcile, so a second call over the same state
// must be a no-op rather than an error or a duplicate event.
func TestReclaimOrphanedRackPVCs_Idempotent(t *testing.T) {
	scheme := rollingRestartScheme(t)
	cluster := pvcReclaimCluster(true)
	stsName := utils.StatefulSetName(cluster.Name, pvcReclaimRackID)

	recorder := record.NewFakeRecorder(8)
	r := &AerospikeClusterReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			pvcReclaimPod(cluster.Name, 0),
			pvcReclaimPVC(cluster.Name, 0),
			pvcReclaimPVC(cluster.Name, 1),
		).Build(),
		Scheme:   scheme,
		Recorder: recorder,
	}

	replicas := int32(1)
	for pass := 1; pass <= 3; pass++ {
		r.reclaimOrphanedRackPVCs(context.Background(), cluster,
			pvcReclaimRackID, stsName, &replicas, cluster.Spec.Storage)
	}

	present := pvcNames(t, r.Client, cluster.Name)
	if !present[fmt.Sprintf("data-%s-0", stsName)] {
		t.Error("in-use PVC at ordinal 0 was deleted")
	}
	if present[fmt.Sprintf("data-%s-1", stsName)] {
		t.Error("orphaned PVC at ordinal 1 was not reclaimed")
	}

	// Exactly one cleanup event: the first pass deletes, the rest find nothing.
	events := 0
	for {
		select {
		case <-recorder.Events:
			events++
			continue
		default:
		}
		break
	}
	if events != 1 {
		t.Errorf("recorded %d events over 3 passes, want 1", events)
	}
}

func TestRackPodOrdinal(t *testing.T) {
	tests := []struct {
		podName string
		want    int
		wantOK  bool
	}{
		{"demo-0-0", 0, true},
		{"demo-0-7", 7, true},
		{"demo-0-12", 12, true},
		{"my-cluster-2-3", 3, true},
		// The ordinal is the segment after the LAST dash, so a leading '-' is
		// always consumed as the separator and the parsed value can never be
		// negative. Pinned here because the reclamation guard's lower bound
		// depends on it.
		{"demo-0--1", 1, true},
		// Fail closed: none of these may be read as ordinal 0.
		{"demo-0-abc", 0, false},
		{"demo-0-", 0, false},
		{"nodashes", 0, false},
		{"", 0, false},
	}

	for _, tc := range tests {
		t.Run(tc.podName, func(t *testing.T) {
			got, ok := rackPodOrdinal(tc.podName)
			if ok != tc.wantOK {
				t.Fatalf("rackPodOrdinal(%q) ok = %v, want %v", tc.podName, ok, tc.wantOK)
			}
			if ok && got != tc.want {
				t.Errorf("rackPodOrdinal(%q) = %d, want %d", tc.podName, got, tc.want)
			}
		})
	}
}
