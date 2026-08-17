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
// Reclamation is destructive, so the tests below are split into two groups:
// those that pin that an orphan IS reclaimed, and a larger set that pin every
// way a PVC must be REJECTED. The rejection set is the one that matters.

const (
	pvcReclaimRackID = 0
	pvcReclaimStsUID = types.UID("sts-uid-demo-0")
)

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

// pvcReclaimSts builds the StatefulSet the reclaim predicate is evaluated
// against: the observed replica count, the UID PVC ownerReferences are matched
// on, and the VolumeClaimTemplate names a PVC's volume must be one of.
func pvcReclaimSts(clusterName string, replicas *int32, vctNames ...string) *appsv1.StatefulSet {
	vcts := make([]corev1.PersistentVolumeClaim, 0, len(vctNames))
	for _, name := range vctNames {
		vcts = append(vcts, corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: name},
		})
	}
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.StatefulSetName(clusterName, pvcReclaimRackID),
			Namespace: ctrlTestNamespace,
			UID:       pvcReclaimStsUID,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:             replicas,
			VolumeClaimTemplates: vcts,
		},
	}
	// A settled status: ObservedGeneration == Generation (both 0 here) and
	// status.replicas == spec.replicas. This is what kube-controller-manager
	// reports for a converged StatefulSet, and what the reclaim status gate
	// requires. Tests that need an unsettled StatefulSet call withStatusReplicas.
	if replicas != nil {
		sts.Status.Replicas = *replicas
	}
	return sts
}

// withStatusReplicas overrides status.replicas so a test can model a
// StatefulSet that still owns pods it has not released — the shape
// kube-controller-manager reports while scaled-down pods are terminating, and
// the one that must defer reclamation.
func withStatusReplicas(sts *appsv1.StatefulSet, statusReplicas int32) *appsv1.StatefulSet {
	sts.Status.Replicas = statusReplicas
	return sts
}

func int32Ptr(v int32) *int32 { return &v }

// pvcReclaimPVCName builds a StatefulSet-style PVC name:
// <volume>-<stsName>-<ordinal>.
func pvcReclaimPVCName(clusterName, volume string, ordinal int) string {
	return fmt.Sprintf("%s-%s-%d", volume, utils.StatefulSetName(clusterName, pvcReclaimRackID), ordinal)
}

// pvcReclaimPVC builds a PVC carrying the cluster labels the operator stamps
// onto VolumeClaimTemplates, and NO ownerReferences — which is the normal shape
// for a StatefulSet volumeClaimTemplate PVC, since the StatefulSet controller
// only adds one when persistentVolumeClaimRetentionPolicy is set to Delete and
// this operator never sets that field.
func pvcReclaimPVC(clusterName string, ordinal int) *corev1.PersistentVolumeClaim {
	return pvcReclaimPVCNamed(clusterName, pvcReclaimPVCName(clusterName, "data", ordinal), true)
}

func pvcReclaimPVCNamed(clusterName, name string, labelled bool) *corev1.PersistentVolumeClaim {
	var labels map[string]string
	if labelled {
		labels = utils.LabelsForCluster(clusterName)
	}
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ctrlTestNamespace,
			Labels:    labels,
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

func withOwnerUID(pvc *corev1.PersistentVolumeClaim, uid types.UID) *corev1.PersistentVolumeClaim {
	pvc.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "StatefulSet",
		Name:       "some-statefulset",
		UID:        uid,
	}}
	return pvc
}

// pvcReclaimPod builds a rack pod named <stsName>-<ordinal> carrying the rack
// labels listRackPods selects on.
func pvcReclaimPod(clusterName string, ordinal int) *corev1.Pod {
	stsName := utils.StatefulSetName(clusterName, pvcReclaimRackID)
	return pvcReclaimNamedPod(clusterName, fmt.Sprintf("%s-%d", stsName, ordinal), true)
}

func pvcReclaimNamedPod(clusterName, podName string, rackLabelled bool) *corev1.Pod {
	labels := utils.LabelsForCluster(clusterName)
	if rackLabelled {
		labels = utils.LabelsForRack(clusterName, pvcReclaimRackID)
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: ctrlTestNamespace,
			Labels:    labels,
		},
	}
}

func pvcExists(t *testing.T, c client.Client, name string) bool {
	t.Helper()
	err := c.Get(context.Background(),
		types.NamespacedName{Name: name, Namespace: ctrlTestNamespace},
		&corev1.PersistentVolumeClaim{})
	switch {
	case err == nil:
		return true
	case apierrors.IsNotFound(err):
		return false
	default:
		t.Fatalf("Get PVC %s: %v", name, err)
		return false
	}
}

func pvcReclaimReconciler(scheme *runtime.Scheme, objs ...client.Object) *AerospikeClusterReconciler {
	return &AerospikeClusterReconciler{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(8),
	}
}

// --- the orphan IS reclaimed ---

// TestReconcileStatefulSet_ReclaimsOrphanedPVCsOnConvergedPass is the
// regression test for the leak. The StatefulSet has already been scaled down to
// 1 replica, the removed pod is gone, and both hashes match — so
// reconcileStatefulSet takes the `!needsUpdate` early return, which is exactly
// where the old code stopped being able to reclaim anything.
func TestReconcileStatefulSet_ReclaimsOrphanedPVCsOnConvergedPass(t *testing.T) {
	scheme := rollingRestartScheme(t)

	cluster := pvcReclaimCluster(true)
	rack := &ackov1alpha1.Rack{ID: pvcReclaimRackID}

	const configHash = "cfg-SAME"
	podSpecHash := computePodSpecHash(cluster, rack)

	sts := pvcReclaimSts(cluster.Name, int32Ptr(1), "data")
	sts.Spec.Template = corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				utils.ConfigHashAnnotation:  configHash,
				utils.PodSpecHashAnnotation: podSpecHash,
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

	if !pvcExists(t, r.Client, pvcReclaimPVCName(cluster.Name, "data", 0)) {
		t.Error("PVC at ordinal 0 was deleted; it is in use by a running pod")
	}
	if pvcExists(t, r.Client, pvcReclaimPVCName(cluster.Name, "data", 1)) {
		t.Error("orphaned PVC at ordinal 1 was not reclaimed on the converged pass")
	}
}

// TestReclaimOrphanedRackPVCs_SelectsPVCWithNoOwnerReferences documents the
// ownership contract deliberately: a PVC with no ownerReferences at all IS
// reclaimable, because that is the default shape of every StatefulSet
// volumeClaimTemplate PVC. persistentVolumeClaimRetentionPolicy defaults to
// Retain/Retain ("retained until manually deleted") and this operator never
// sets it, so the StatefulSet controller stamps no ownerReference. Requiring one
// would reclaim nothing at all and silently disable the fix.
func TestReclaimOrphanedRackPVCs_SelectsPVCWithNoOwnerReferences(t *testing.T) {
	scheme := rollingRestartScheme(t)
	cluster := pvcReclaimCluster(true)

	orphan := pvcReclaimPVC(cluster.Name, 1)
	if len(orphan.OwnerReferences) != 0 {
		t.Fatalf("fixture should have no ownerReferences, got %v", orphan.OwnerReferences)
	}

	r := pvcReclaimReconciler(scheme, pvcReclaimPod(cluster.Name, 0), orphan)
	r.reclaimOrphanedRackPVCs(context.Background(), cluster, pvcReclaimRackID,
		pvcReclaimSts(cluster.Name, int32Ptr(1), "data"), 1, cluster.Spec.Storage)

	if pvcExists(t, r.Client, orphan.Name) {
		t.Error("orphan with no ownerReferences was not reclaimed; the default " +
			"StatefulSet PVC shape must remain reclaimable")
	}
}

// TestReclaimOrphanedRackPVCs_SelectsPVCOwnedByThisStatefulSet covers the other
// accepted shape: an ownerReference that matches the StatefulSet's UID, which is
// what a cluster running persistentVolumeClaimRetentionPolicy: whenDeleted=Delete
// would produce.
func TestReclaimOrphanedRackPVCs_SelectsPVCOwnedByThisStatefulSet(t *testing.T) {
	scheme := rollingRestartScheme(t)
	cluster := pvcReclaimCluster(true)

	orphan := withOwnerUID(pvcReclaimPVC(cluster.Name, 1), pvcReclaimStsUID)

	r := pvcReclaimReconciler(scheme, pvcReclaimPod(cluster.Name, 0), orphan)
	r.reclaimOrphanedRackPVCs(context.Background(), cluster, pvcReclaimRackID,
		pvcReclaimSts(cluster.Name, int32Ptr(1), "data"), 1, cluster.Spec.Storage)

	if pvcExists(t, r.Client, orphan.Name) {
		t.Error("orphan owned by this StatefulSet was not reclaimed")
	}
}

// --- every way a PVC must be REJECTED ---

// TestReclaimOrphanedRackPVCs_NeverSelectsInUsePVC is the core safety test.
// Deleting a PVC a running Aerospike node has mounted destroys that node's data,
// so only ordinals at or above the StatefulSet's own replica count may ever be
// considered — never the desired rack size, which during a batched scale-down is
// lower than the count the StatefulSet is actually running.
func TestReclaimOrphanedRackPVCs_NeverSelectsInUsePVC(t *testing.T) {
	tests := []struct {
		name         string
		specReplicas int32
		podOrdinals  []int
		pvcOrdinals  []int
		wantDeleted  []int
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
			// Previously this case asserted that ordinal 3 was reclaimed with no
			// pods observed at all. That encoded a weaker contract than the code
			// can safely offer: an empty pod list is indistinguishable from a
			// label query that is not seeing live pods, and the per-pod ordinal
			// check then passes vacuously. The pass must now defer entirely.
			name:         "no pods observed defers rather than reclaiming",
			specReplicas: 3,
			podOrdinals:  nil,
			pvcOrdinals:  []int{0, 1, 2, 3},
			wantDeleted:  nil,
		},
		{
			name:         "fewer pods than replicas defers",
			specReplicas: 3,
			podOrdinals:  []int{0, 1},
			pvcOrdinals:  []int{0, 1, 2, 3},
			wantDeleted:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scheme := rollingRestartScheme(t)
			cluster := pvcReclaimCluster(true)

			objs := make([]client.Object, 0, len(tc.podOrdinals)+len(tc.pvcOrdinals))
			for _, ordinal := range tc.podOrdinals {
				objs = append(objs, pvcReclaimPod(cluster.Name, ordinal))
			}
			for _, ordinal := range tc.pvcOrdinals {
				objs = append(objs, pvcReclaimPVC(cluster.Name, ordinal))
			}

			r := pvcReclaimReconciler(scheme, objs...)
			r.reclaimOrphanedRackPVCs(context.Background(), cluster, pvcReclaimRackID,
				pvcReclaimSts(cluster.Name, int32Ptr(tc.specReplicas), "data"),
				tc.specReplicas, cluster.Spec.Storage)

			shouldBeGone := map[int]bool{}
			for _, ordinal := range tc.wantDeleted {
				shouldBeGone[ordinal] = true
			}

			for _, ordinal := range tc.pvcOrdinals {
				name := pvcReclaimPVCName(cluster.Name, "data", ordinal)
				present := pvcExists(t, r.Client, name)
				if shouldBeGone[ordinal] && present {
					t.Errorf("PVC %s should have been reclaimed (replicas=%d)", name, tc.specReplicas)
				}
				if !shouldBeGone[ordinal] && !present {
					t.Errorf("PVC %s was deleted but must have been preserved (replicas=%d)",
						name, tc.specReplicas)
				}
			}
		})
	}
}

// TestReclaimOrphanedRackPVCs_DefersWhenPodsMissingRackLabel is the fail-open
// counterexample. listRackPods selects on LabelsForRack — the cluster labels
// plus RackLabel. Pods carrying only the cluster labels are invisible to it, so
// the per-pod ordinal check passes over an empty list while live pods sit above
// the replica count. Requiring len(pods) == spec.replicas closes that: the pod
// list is load-bearing, not a second layer.
func TestReclaimOrphanedRackPVCs_DefersWhenPodsMissingRackLabel(t *testing.T) {
	scheme := rollingRestartScheme(t)
	cluster := pvcReclaimCluster(true)
	stsName := utils.StatefulSetName(cluster.Name, pvcReclaimRackID)

	r := pvcReclaimReconciler(scheme,
		// Live pods at ordinals 0-2, none visible to listRackPods.
		pvcReclaimNamedPod(cluster.Name, fmt.Sprintf("%s-0", stsName), false),
		pvcReclaimNamedPod(cluster.Name, fmt.Sprintf("%s-1", stsName), false),
		pvcReclaimNamedPod(cluster.Name, fmt.Sprintf("%s-2", stsName), false),
		pvcReclaimPVC(cluster.Name, 0),
		pvcReclaimPVC(cluster.Name, 1),
		pvcReclaimPVC(cluster.Name, 2),
	)

	// The StatefulSet is mid-scale-down: replicas already reads 1 while pods
	// 1 and 2 are still running.
	r.reclaimOrphanedRackPVCs(context.Background(), cluster, pvcReclaimRackID,
		pvcReclaimSts(cluster.Name, int32Ptr(1), "data"), 1, cluster.Spec.Storage)

	for ordinal := range 3 {
		name := pvcReclaimPVCName(cluster.Name, "data", ordinal)
		if !pvcExists(t, r.Client, name) {
			t.Errorf("PVC %s was deleted while a live pod at that ordinal exists "+
				"but is invisible to the rack label query", name)
		}
	}
}

// TestReclaimOrphanedRackPVCs_NeverSelectsForeignOwnedPVC pins the
// ownerReference check. A PVC that names some other owner — a sibling
// StatefulSet, another cluster, a hand-created claim — must never be reclaimed
// even though its name matches the ordinal pattern and its labels match.
func TestReclaimOrphanedRackPVCs_NeverSelectsForeignOwnedPVC(t *testing.T) {
	scheme := rollingRestartScheme(t)
	cluster := pvcReclaimCluster(true)

	foreign := withOwnerUID(pvcReclaimPVC(cluster.Name, 1), types.UID("some-other-statefulset-uid"))

	r := pvcReclaimReconciler(scheme, pvcReclaimPod(cluster.Name, 0), foreign)
	r.reclaimOrphanedRackPVCs(context.Background(), cluster, pvcReclaimRackID,
		pvcReclaimSts(cluster.Name, int32Ptr(1), "data"), 1, cluster.Spec.Storage)

	if !pvcExists(t, r.Client, foreign.Name) {
		t.Error("PVC owned by a foreign UID was reclaimed")
	}
}

// TestReclaimOrphanedRackPVCs_NeverSelectsUnlabelledPVC proves reclamation does
// not inherit GetPVCsForStatefulSet's namespace-wide fallback, where a name
// substring is the only ownership signal. That fallback exists for cluster
// deletion; reclamation runs on every reconcile of every rack and must not use
// it.
func TestReclaimOrphanedRackPVCs_NeverSelectsUnlabelledPVC(t *testing.T) {
	scheme := rollingRestartScheme(t)
	cluster := pvcReclaimCluster(true)

	// Name matches the ordinal pattern exactly, but carries no cluster labels.
	unlabelled := pvcReclaimPVCNamed(cluster.Name,
		pvcReclaimPVCName(cluster.Name, "data", 9), false)

	r := pvcReclaimReconciler(scheme, pvcReclaimPod(cluster.Name, 0), unlabelled)
	r.reclaimOrphanedRackPVCs(context.Background(), cluster, pvcReclaimRackID,
		pvcReclaimSts(cluster.Name, int32Ptr(1), "data"), 1, cluster.Spec.Storage)

	if !pvcExists(t, r.Client, unlabelled.Name) {
		t.Error("unlabelled PVC was reclaimed through a name-only ownership test")
	}
}

// TestReclaimOrphanedRackPVCs_NeverSelectsVolumeNotInStatefulSet pins the
// VolumeClaimTemplate check against the case that actually needs it: spec drift.
//
// VolumeClaimTemplates are immutable after StatefulSet creation, so a user who
// adds a cascadeDelete volume to spec.storage leaves the live StatefulSet with
// the old VCT set. The cascadeDelete filter alone would then accept a PVC for
// the newly-declared volume even though this StatefulSet never provisioned it —
// so the volume name is checked against the live object's own templates.
func TestReclaimOrphanedRackPVCs_NeverSelectsVolumeNotInStatefulSet(t *testing.T) {
	scheme := rollingRestartScheme(t)
	cluster := pvcReclaimCluster(true)

	// spec.storage now declares a second cascadeDelete volume, "scratch" ...
	cascade := true
	cluster.Spec.Storage.Volumes = append(cluster.Spec.Storage.Volumes, ackov1alpha1.VolumeSpec{
		Name: "scratch",
		Source: ackov1alpha1.VolumeSource{
			PersistentVolume: &ackov1alpha1.PersistentVolumeSpec{Size: "5Gi"},
		},
		CascadeDelete: &cascade,
	})

	// ... but the live StatefulSet's VCTs still only carry "data", so this claim
	// belongs to something else despite matching labels, ordinal and cascade.
	foreignVol := pvcReclaimPVCNamed(cluster.Name,
		pvcReclaimPVCName(cluster.Name, "scratch", 1), true)

	r := pvcReclaimReconciler(scheme, pvcReclaimPod(cluster.Name, 0), foreignVol)
	r.reclaimOrphanedRackPVCs(context.Background(), cluster, pvcReclaimRackID,
		pvcReclaimSts(cluster.Name, int32Ptr(1), "data"), 1, cluster.Spec.Storage)

	if !pvcExists(t, r.Client, foreignVol.Name) {
		t.Error("PVC whose volume is not a VolumeClaimTemplate of this StatefulSet was reclaimed")
	}
}

// TestReclaimOrphanedRackPVCs_NeverSelectsMalformedOrdinal covers a PVC whose
// trailing segment does not parse as an ordinal. It must be skipped, not
// treated as ordinal 0 or as an orphan.
func TestReclaimOrphanedRackPVCs_NeverSelectsMalformedOrdinal(t *testing.T) {
	scheme := rollingRestartScheme(t)
	cluster := pvcReclaimCluster(true)
	stsName := utils.StatefulSetName(cluster.Name, pvcReclaimRackID)

	malformed := pvcReclaimPVCNamed(cluster.Name, fmt.Sprintf("data-%s-abc", stsName), true)

	r := pvcReclaimReconciler(scheme, pvcReclaimPod(cluster.Name, 0), malformed)
	r.reclaimOrphanedRackPVCs(context.Background(), cluster, pvcReclaimRackID,
		pvcReclaimSts(cluster.Name, int32Ptr(1), "data"), 1, cluster.Spec.Storage)

	if !pvcExists(t, r.Client, malformed.Name) {
		t.Error("PVC with a malformed ordinal was reclaimed")
	}
}

// TestReclaimOrphanedRackPVCs_DefersWhilePodAboveReplicaCountExists covers the
// mid-scale-down window: the pod being removed is still terminating and still
// has its volume mounted.
func TestReclaimOrphanedRackPVCs_DefersWhilePodAboveReplicaCountExists(t *testing.T) {
	scheme := rollingRestartScheme(t)
	cluster := pvcReclaimCluster(true)

	r := pvcReclaimReconciler(scheme,
		pvcReclaimPod(cluster.Name, 0),
		pvcReclaimPod(cluster.Name, 1), // still terminating
		pvcReclaimPVC(cluster.Name, 0),
		pvcReclaimPVC(cluster.Name, 1),
	)

	r.reclaimOrphanedRackPVCs(context.Background(), cluster, pvcReclaimRackID,
		pvcReclaimSts(cluster.Name, int32Ptr(1), "data"), 1, cluster.Spec.Storage)

	for ordinal := range 2 {
		name := pvcReclaimPVCName(cluster.Name, "data", ordinal)
		if !pvcExists(t, r.Client, name) {
			t.Errorf("PVC %s was deleted while a pod above the replica count is still terminating", name)
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
		pvcReclaimNamedPod(cluster.Name, stsName+"-not-a-number", true),
		pvcReclaimPVC(cluster.Name, 0),
		pvcReclaimPVC(cluster.Name, 1),
	)

	r.reclaimOrphanedRackPVCs(context.Background(), cluster, pvcReclaimRackID,
		pvcReclaimSts(cluster.Name, int32Ptr(1), "data"), 1, cluster.Spec.Storage)

	if !pvcExists(t, r.Client, pvcReclaimPVCName(cluster.Name, "data", 1)) {
		t.Error("PVC was reclaimed despite a rack pod with an unparseable ordinal")
	}
}

// TestReclaimOrphanedRackPVCs_SkipsWhenReplicaCountUnknown guards the nil case.
// Treating a nil spec.replicas as 0 would make every PVC in the rack an orphan
// candidate — including every in-use one.
func TestReclaimOrphanedRackPVCs_SkipsWhenReplicaCountUnknown(t *testing.T) {
	scheme := rollingRestartScheme(t)
	cluster := pvcReclaimCluster(true)

	r := pvcReclaimReconciler(scheme,
		pvcReclaimPVC(cluster.Name, 0),
		pvcReclaimPVC(cluster.Name, 1),
	)

	r.reclaimOrphanedRackPVCs(context.Background(), cluster, pvcReclaimRackID,
		pvcReclaimSts(cluster.Name, nil, "data"), 1, cluster.Spec.Storage)

	for ordinal := range 2 {
		name := pvcReclaimPVCName(cluster.Name, "data", ordinal)
		if !pvcExists(t, r.Client, name) {
			t.Errorf("PVC %s was deleted with an unknown replica count", name)
		}
	}
}

// TestReclaimOrphanedRackPVCs_SkipsAtZeroReplicas keeps whole-rack teardown out
// of this path: at zero replicas every ordinal is >= the replica count, and
// deleting by ordinal would duplicate the rack-removal decision without its
// guards. Documented in the PR as a known, deliberate gap.
func TestReclaimOrphanedRackPVCs_SkipsAtZeroReplicas(t *testing.T) {
	scheme := rollingRestartScheme(t)
	cluster := pvcReclaimCluster(true)

	r := pvcReclaimReconciler(scheme,
		pvcReclaimPVC(cluster.Name, 0),
		pvcReclaimPVC(cluster.Name, 1),
	)

	r.reclaimOrphanedRackPVCs(context.Background(), cluster, pvcReclaimRackID,
		pvcReclaimSts(cluster.Name, int32Ptr(0), "data"), 0, cluster.Spec.Storage)

	for ordinal := range 2 {
		name := pvcReclaimPVCName(cluster.Name, "data", ordinal)
		if !pvcExists(t, r.Client, name) {
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

	r.reclaimOrphanedRackPVCs(context.Background(), cluster, pvcReclaimRackID,
		pvcReclaimSts(cluster.Name, int32Ptr(1), "data"), 1, cluster.Spec.Storage)

	if lists != 0 {
		t.Errorf("issued %d List call(s) for a cluster with no cascadeDelete volume, want 0", lists)
	}

	for ordinal := range 2 {
		name := pvcReclaimPVCName(cluster.Name, "data", ordinal)
		if !pvcExists(t, r.Client, name) {
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

	sts := pvcReclaimSts(cluster.Name, int32Ptr(1), "data")
	for pass := 1; pass <= 3; pass++ {
		r.reclaimOrphanedRackPVCs(context.Background(), cluster, pvcReclaimRackID,
			sts, 1, cluster.Spec.Storage)
	}

	if !pvcExists(t, r.Client, pvcReclaimPVCName(cluster.Name, "data", 0)) {
		t.Error("in-use PVC at ordinal 0 was deleted")
	}
	if pvcExists(t, r.Client, pvcReclaimPVCName(cluster.Name, "data", 1)) {
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
		// negative. Pinned because the reclamation guard's lower bound depends
		// on it.
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

// --- regressions ported from independent verification of this PR ---
//
// Both hazards below were introduced by moving reclamation to the top of
// reconcileStatefulSet, and both were demonstrated by a reviewer with a failing
// test before being fixed. They are kept here so the hazards cannot return
// silently.

// TestReconcileStatefulSet_DoesNotReclaimOnExternalScaleDown covers an external
// write that lowered spec.replicas behind the operator's back — `kubectl scale
// sts`, an HPA aimed at the StatefulSet instead of the CR, GitOps drift, a
// backup restore.
//
// Reclamation sits above the scale-down branch, so isMigrationInProgress and the
// quiesce path never run for this transition. Before the rackSize guard the
// operator deleted the volumes of every ordinal above the externally-lowered
// count and then, on the same pass, scaled the rack back up onto blank devices —
// a destructive behaviour that did not exist before this PR, for a scale-down
// the CR never asked for.
func TestReconcileStatefulSet_DoesNotReclaimOnExternalScaleDown(t *testing.T) {
	scheme := rollingRestartScheme(t)

	cluster := pvcReclaimCluster(true)
	cluster.Spec.Size = 3
	rack := &ackov1alpha1.Rack{ID: pvcReclaimRackID}

	const configHash = "cfg-SAME"
	podSpecHash := computePodSpecHash(cluster, rack)

	// Someone scaled the StatefulSet to 1. The removed pods have finished
	// terminating, so the rack looks "converged" at 1 by every local signal.
	sts := pvcReclaimSts(cluster.Name, int32Ptr(1), "data")
	sts.Spec.Template = corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				utils.ConfigHashAnnotation:  configHash,
				utils.PodSpecHashAnnotation: podSpecHash,
			},
		},
	}

	r := pvcReclaimReconciler(scheme,
		sts,
		pvcReclaimPod(cluster.Name, 0),
		pvcReclaimPVC(cluster.Name, 0),
		pvcReclaimPVC(cluster.Name, 1), // holds node 1's Aerospike data
		pvcReclaimPVC(cluster.Name, 2), // holds node 2's Aerospike data
	)

	// rackSize = 3: the operator is about to scale the rack back up to the size
	// the CR asks for.
	if _, err := r.reconcileStatefulSet(
		context.Background(), cluster, rack, nil, configHash, 3); err != nil {
		t.Fatalf("reconcileStatefulSet() error = %v", err)
	}

	for _, ordinal := range []int{1, 2} {
		name := pvcReclaimPVCName(cluster.Name, "data", ordinal)
		if !pvcExists(t, r.Client, name) {
			t.Errorf("PVC %s was destroyed on the pass that scales the rack back up; "+
				"the CR never asked for a scale-down", name)
		}
	}

	// And the rack really is scaled back up, so this is not passing by accident
	// of the reconcile bailing out early.
	after := &appsv1.StatefulSet{}
	if err := r.Get(context.Background(),
		types.NamespacedName{Name: sts.Name, Namespace: ctrlTestNamespace}, after); err != nil {
		t.Fatalf("get sts: %v", err)
	}
	if after.Spec.Replicas == nil || *after.Spec.Replicas != 3 {
		t.Errorf("expected the rack to be scaled back up to 3, got %v", after.Spec.Replicas)
	}
}

// TestReconcileStatefulSet_DoesNotEatPreProvisionedScaleUpPVCs is the same guard
// from the operator's other side: an admin pre-provisions PVCs at the ordinals a
// scale-up will use (seeding nodes from restored PVs), then raises size.
// Reclamation runs before the scale-up patch, so without the rackSize guard it
// consumed exactly the claims the admin staged.
func TestReconcileStatefulSet_DoesNotEatPreProvisionedScaleUpPVCs(t *testing.T) {
	scheme := rollingRestartScheme(t)

	cluster := pvcReclaimCluster(true)
	cluster.Spec.Size = 3
	rack := &ackov1alpha1.Rack{ID: pvcReclaimRackID}

	const configHash = "cfg-SAME"
	podSpecHash := computePodSpecHash(cluster, rack)

	sts := pvcReclaimSts(cluster.Name, int32Ptr(1), "data")
	sts.Spec.Template = corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				utils.ConfigHashAnnotation:  configHash,
				utils.PodSpecHashAnnotation: podSpecHash,
			},
		},
	}

	r := pvcReclaimReconciler(scheme,
		sts,
		pvcReclaimPod(cluster.Name, 0),
		pvcReclaimPVC(cluster.Name, 0),
		// Hand-created, bound to restored PVs, labelled like the operator's own.
		pvcReclaimPVC(cluster.Name, 1),
		pvcReclaimPVC(cluster.Name, 2),
	)

	if _, err := r.reconcileStatefulSet(
		context.Background(), cluster, rack, nil, configHash, 3); err != nil {
		t.Fatalf("reconcileStatefulSet() error = %v", err)
	}

	for _, ordinal := range []int{1, 2} {
		name := pvcReclaimPVCName(cluster.Name, "data", ordinal)
		if !pvcExists(t, r.Client, name) {
			t.Errorf("pre-provisioned PVC %s was destroyed by the pre-scale-up reclaim", name)
		}
	}
}

// TestReclaimOrphanedRackPVCs_DefersWhenLivePodsAreInvisibleToRackQuery covers
// the pod-gate bypass. len(rackPods) == spec.replicas only catches a *wrong*
// count, so hiding pods until the count comes out right defeats it.
//
// This is reachable without an attacker: the StatefulSet's Selector is
// SelectorLabelsForCluster, which carries no rack label, so acko.io/rack is not
// selector-enforced — and podutil.BuildPodTemplateSpec lets
// spec.podSpec.metadata.labels overwrite it. A lagging pod informer produces the
// same shape. The fix is to gate on the StatefulSet's own status, which
// kube-controller-manager computes from the StatefulSet's selector and which
// arrives on the same object read as spec.replicas.
func TestReclaimOrphanedRackPVCs_DefersWhenLivePodsAreInvisibleToRackQuery(t *testing.T) {
	stsName := utils.StatefulSetName("demo", pvcReclaimRackID)

	tests := []struct {
		name string
		// pods are the objects that exist; hiddenFromList are removed from every
		// List result to model either a mislabelled pod or a stale informer.
		visiblePods []int
		hiddenPods  []int
	}{
		{
			// Pods 1 and 2 are live but carry no rack label, so listRackPods
			// returns exactly one pod and the count check passes.
			name:        "pods mislabelled so the visible count matches",
			visiblePods: []int{0},
			hiddenPods:  []int{1, 2},
		},
		{
			// Same shape via a lagging pod informer.
			name:        "pods hidden by a stale informer",
			visiblePods: []int{0},
			hiddenPods:  []int{1, 2},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scheme := rollingRestartScheme(t)
			cluster := pvcReclaimCluster(true)

			objs := make([]client.Object, 0, len(tc.visiblePods)+len(tc.hiddenPods)+3)
			for _, o := range tc.visiblePods {
				objs = append(objs, pvcReclaimPod(cluster.Name, o))
			}
			for _, o := range tc.hiddenPods {
				objs = append(objs, pvcReclaimPod(cluster.Name, o))
			}
			for ordinal := range 3 {
				objs = append(objs, pvcReclaimPVC(cluster.Name, ordinal))
			}

			base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
			hidden := map[string]bool{}
			for _, o := range tc.hiddenPods {
				hidden[fmt.Sprintf("%s-%d", stsName, o)] = true
			}
			cl := interceptor.NewClient(base, interceptor.Funcs{
				List: func(ctx context.Context, c client.WithWatch,
					list client.ObjectList, opts ...client.ListOption) error {
					if err := c.List(ctx, list, opts...); err != nil {
						return err
					}
					pl, ok := list.(*corev1.PodList)
					if !ok {
						return nil
					}
					kept := pl.Items[:0]
					for i := range pl.Items {
						if !hidden[pl.Items[i].Name] {
							kept = append(kept, pl.Items[i])
						}
					}
					pl.Items = kept
					return nil
				},
			})

			r := &AerospikeClusterReconciler{
				Client: cl, Scheme: scheme, Recorder: record.NewFakeRecorder(8),
			}

			// spec.replicas is 1, but kube-controller-manager still counts all
			// three pods via the StatefulSet's own selector, so status.replicas
			// is 3. That mismatch is what defers the pass.
			sts := withStatusReplicas(pvcReclaimSts(cluster.Name, int32Ptr(1), "data"), 3)

			r.reclaimOrphanedRackPVCs(context.Background(), cluster, pvcReclaimRackID,
				sts, 1, cluster.Spec.Storage)

			for ordinal := range 3 {
				name := pvcReclaimPVCName(cluster.Name, "data", ordinal)
				if !pvcExists(t, r.Client, name) {
					t.Errorf("PVC %s deleted while its pod is live but invisible to the rack query", name)
				}
			}
		})
	}
}

// TestReclaimOrphanedRackPVCs_DefersUntilStatusCatchesUp pins the
// ObservedGeneration half of the status gate: immediately after the operator
// patches spec.replicas the StatefulSet controller has not acted yet, so no
// conclusion may be drawn from status.replicas.
func TestReclaimOrphanedRackPVCs_DefersUntilStatusCatchesUp(t *testing.T) {
	scheme := rollingRestartScheme(t)
	cluster := pvcReclaimCluster(true)

	r := pvcReclaimReconciler(scheme,
		pvcReclaimPod(cluster.Name, 0),
		pvcReclaimPVC(cluster.Name, 0),
		pvcReclaimPVC(cluster.Name, 1),
	)

	sts := pvcReclaimSts(cluster.Name, int32Ptr(1), "data")
	sts.Generation = 7
	sts.Status.ObservedGeneration = 6 // controller has not caught up

	r.reclaimOrphanedRackPVCs(context.Background(), cluster, pvcReclaimRackID,
		sts, 1, cluster.Spec.Storage)

	if !pvcExists(t, r.Client, pvcReclaimPVCName(cluster.Name, "data", 1)) {
		t.Error("PVC reclaimed while the StatefulSet status had not caught up with its spec")
	}
}

// TestReclaimOrphanedRackPVCs_NeverSelectsPVCMissingComponentLabels closes the
// gap a reviewer found by constructing a PVC that satisfied every other guard:
// correct name, correct ordinal, nil ownerReferences, and the two labels the
// legacy getter checks. The reclaim listing now matches all four labels
// LabelsForCluster stamps, so a claim carrying only name+instance is not ours.
func TestReclaimOrphanedRackPVCs_NeverSelectsPVCMissingComponentLabels(t *testing.T) {
	scheme := rollingRestartScheme(t)
	cluster := pvcReclaimCluster(true)

	foreign := pvcReclaimPVCNamed(cluster.Name,
		pvcReclaimPVCName(cluster.Name, "data", 1), false)
	foreign.Labels = map[string]string{
		utils.AppLabel:      "aerospike-cluster",
		utils.InstanceLabel: cluster.Name,
		// no component, no managed-by
	}

	r := pvcReclaimReconciler(scheme, pvcReclaimPod(cluster.Name, 0), foreign)
	r.reclaimOrphanedRackPVCs(context.Background(), cluster, pvcReclaimRackID,
		pvcReclaimSts(cluster.Name, int32Ptr(1), "data"), 1, cluster.Spec.Storage)

	if !pvcExists(t, r.Client, foreign.Name) {
		t.Error("PVC carrying only app/instance labels was reclaimed; " +
			"component and managed-by must also match")
	}
}
