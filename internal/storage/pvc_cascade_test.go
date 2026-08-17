package storage

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

const (
	testNamespace   = "default"
	testClusterName = "my-cluster"
	testStsName     = "my-cluster-0"
)

func newPVC(name string) *corev1.PersistentVolumeClaim {
	return newPVCForCluster(name, testClusterName)
}

// newPVCForCluster builds a PVC carrying the operator's standard labels for the
// given cluster, so label-scoped queries (app.kubernetes.io/instance) match it.
func newPVCForCluster(name, clusterName string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":     "aerospike-cluster",
				"app.kubernetes.io/instance": clusterName,
			},
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

func buildFakeClient(objs ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, o := range objs {
		builder = builder.WithObjects(o)
	}
	return builder.Build()
}

func storageSpecWithCascade(cascadeDelete bool) *v1alpha1.AerospikeStorageSpec {
	return &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
				},
				CascadeDelete: &cascadeDelete,
			},
		},
	}
}

func storageSpecMultiVolume(cascadeVol, nonCascadeVol string) *v1alpha1.AerospikeStorageSpec {
	cascadeTrue := true
	cascadeFalse := false
	return &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: cascadeVol,
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
				},
				CascadeDelete: &cascadeTrue,
			},
			{
				Name: nonCascadeVol,
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "5Gi"},
				},
				CascadeDelete: &cascadeFalse,
			},
		},
	}
}

// testStsUID is the UID PVC ownerReferences must match to be reclaimable.
const testStsUID = types.UID("sts-uid-1")

// newTestSts builds the StatefulSet the reclaim predicate is evaluated against.
// It supplies the namespace, the name prefix, the UID and the
// VolumeClaimTemplate names a PVC's volume must be one of.
func newTestSts(vctNames ...string) *appsv1.StatefulSet {
	vcts := make([]corev1.PersistentVolumeClaim, 0, len(vctNames))
	for _, name := range vctNames {
		vcts = append(vcts, corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: name},
		})
	}
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testStsName,
			Namespace: testNamespace,
			UID:       testStsUID,
		},
		Spec: appsv1.StatefulSetSpec{VolumeClaimTemplates: vcts},
	}
}

// --- DeleteOrphanedCascadeDeletePVCs tests ---

func TestDeleteOrphanedCascadeDeletePVCs_DeletesOnlyCascadeOrphans(t *testing.T) {
	// Setup: 3 replicas scaled down to 1
	// PVCs: data-my-cluster-0-{0,1,2} (cascade=true)
	//       logs-my-cluster-0-{0,1,2} (cascade=false)
	spec := storageSpecMultiVolume("data", "logs")

	c := buildFakeClient(
		newPVC("data-"+testStsName+"-0"),
		newPVC("data-"+testStsName+"-1"),
		newPVC("data-"+testStsName+"-2"),
		newPVC("logs-"+testStsName+"-0"),
		newPVC("logs-"+testStsName+"-1"),
		newPVC("logs-"+testStsName+"-2"),
	)

	ctx := context.Background()
	deleted, err := DeleteOrphanedCascadeDeletePVCs(ctx, c, newTestSts("data", "logs"), testClusterName, 1, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should delete data-my-cluster-0-1 and data-my-cluster-0-2 (cascade=true, ordinal >= 1)
	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}

	// Verify remaining PVCs
	remaining := &corev1.PersistentVolumeClaimList{}
	if err := c.List(ctx, remaining, client.InNamespace(testNamespace)); err != nil {
		t.Fatalf("listing PVCs: %v", err)
	}

	// Should have 4 remaining: data-0, logs-0, logs-1, logs-2
	if len(remaining.Items) != 4 {
		t.Errorf("remaining PVCs = %d, want 4", len(remaining.Items))
		for _, pvc := range remaining.Items {
			t.Logf("  remaining: %s", pvc.Name)
		}
	}

	// Verify cascade PVCs for ordinal 0 are preserved
	remainingNames := make(map[string]bool)
	for _, pvc := range remaining.Items {
		remainingNames[pvc.Name] = true
	}
	if !remainingNames["data-"+testStsName+"-0"] {
		t.Error("data PVC for ordinal 0 should be preserved")
	}
	if !remainingNames["logs-"+testStsName+"-0"] {
		t.Error("logs PVC for ordinal 0 should be preserved")
	}
	if !remainingNames["logs-"+testStsName+"-1"] {
		t.Error("logs PVC for ordinal 1 should be preserved (non-cascade)")
	}
	if !remainingNames["logs-"+testStsName+"-2"] {
		t.Error("logs PVC for ordinal 2 should be preserved (non-cascade)")
	}
}

func TestDeleteOrphanedCascadeDeletePVCs_NoCascadeVolumes(t *testing.T) {
	// All volumes have cascadeDelete=false
	spec := storageSpecWithCascade(false)

	c := buildFakeClient(
		newPVC("data-"+testStsName+"-0"),
		newPVC("data-"+testStsName+"-1"),
	)

	ctx := context.Background()
	deleted, err := DeleteOrphanedCascadeDeletePVCs(ctx, c, newTestSts("data"), testClusterName, 1, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 (no cascade volumes)", deleted)
	}

	// All PVCs should remain
	remaining := &corev1.PersistentVolumeClaimList{}
	if err := c.List(ctx, remaining, client.InNamespace(testNamespace)); err != nil {
		t.Fatalf("listing PVCs: %v", err)
	}
	if len(remaining.Items) != 2 {
		t.Errorf("remaining PVCs = %d, want 2", len(remaining.Items))
	}
}

func TestDeleteOrphanedCascadeDeletePVCs_NilStorageSpec(t *testing.T) {
	c := buildFakeClient(
		newPVC("data-" + testStsName + "-0"),
	)

	ctx := context.Background()
	deleted, err := DeleteOrphanedCascadeDeletePVCs(ctx, c, newTestSts("data"), testClusterName, 0, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 (nil storage spec)", deleted)
	}
}

func TestDeleteOrphanedCascadeDeletePVCs_AllCascadeAllOrphans(t *testing.T) {
	// Scale from 3 to 0 with cascadeDelete=true
	spec := storageSpecWithCascade(true)

	c := buildFakeClient(
		newPVC("data-"+testStsName+"-0"),
		newPVC("data-"+testStsName+"-1"),
		newPVC("data-"+testStsName+"-2"),
	)

	ctx := context.Background()
	deleted, err := DeleteOrphanedCascadeDeletePVCs(ctx, c, newTestSts("data"), testClusterName, 0, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != 3 {
		t.Errorf("deleted = %d, want 3", deleted)
	}

	remaining := &corev1.PersistentVolumeClaimList{}
	if err := c.List(ctx, remaining, client.InNamespace(testNamespace)); err != nil {
		t.Fatalf("listing PVCs: %v", err)
	}
	if len(remaining.Items) != 0 {
		t.Errorf("remaining PVCs = %d, want 0", len(remaining.Items))
	}
}

func TestDeleteOrphanedCascadeDeletePVCs_NoOrphans(t *testing.T) {
	// No scale-down: desiredReplicas equals actual
	spec := storageSpecWithCascade(true)

	c := buildFakeClient(
		newPVC("data-"+testStsName+"-0"),
		newPVC("data-"+testStsName+"-1"),
	)

	ctx := context.Background()
	deleted, err := DeleteOrphanedCascadeDeletePVCs(ctx, c, newTestSts("data"), testClusterName, 2, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 (no orphans)", deleted)
	}
}

func TestDeleteOrphanedCascadeDeletePVCs_PolicyFallback(t *testing.T) {
	// Volume has no per-volume cascadeDelete, but global filesystem policy has it
	spec := &v1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			CascadeDelete: boolPtr(true),
		},
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{
						Size:       "10Gi",
						VolumeMode: corev1.PersistentVolumeFilesystem,
					},
				},
				// CascadeDelete is nil — falls back to policy
			},
		},
	}

	c := buildFakeClient(
		newPVC("data-"+testStsName+"-0"),
		newPVC("data-"+testStsName+"-1"),
	)

	ctx := context.Background()
	deleted, err := DeleteOrphanedCascadeDeletePVCs(ctx, c, newTestSts("data"), testClusterName, 1, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// data-my-cluster-0-1 should be deleted (policy cascadeDelete=true, ordinal >= 1)
	if deleted != 1 {
		t.Errorf("deleted = %d, want 1", deleted)
	}
}

// --- DeleteCascadeDeletePVCs tests ---

func TestDeleteCascadeDeletePVCs_OnlyCascadeVolumes(t *testing.T) {
	spec := storageSpecMultiVolume("data", "logs")

	c := buildFakeClient(
		newPVC("data-"+testStsName+"-0"),
		newPVC("data-"+testStsName+"-1"),
		newPVC("logs-"+testStsName+"-0"),
		newPVC("logs-"+testStsName+"-1"),
	)

	ctx := context.Background()
	err := DeleteCascadeDeletePVCs(ctx, c, testNamespace, testClusterName, testStsName, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	remaining := &corev1.PersistentVolumeClaimList{}
	if err := c.List(ctx, remaining, client.InNamespace(testNamespace)); err != nil {
		t.Fatalf("listing PVCs: %v", err)
	}

	// Only logs PVCs should remain (cascadeDelete=false)
	if len(remaining.Items) != 2 {
		t.Errorf("remaining PVCs = %d, want 2", len(remaining.Items))
	}
	for _, pvc := range remaining.Items {
		volName, ok := extractVolumeName(pvc.Name, testStsName)
		if !ok {
			t.Errorf("unexpected PVC name pattern: %s", pvc.Name)
			continue
		}
		if volName != "logs" {
			t.Errorf("non-cascade PVC %s should have been preserved but was deleted", pvc.Name)
		}
	}
}

func TestDeleteCascadeDeletePVCs_NilStorageSpec(t *testing.T) {
	c := buildFakeClient(
		newPVC("data-" + testStsName + "-0"),
	)

	ctx := context.Background()
	err := DeleteCascadeDeletePVCs(ctx, c, testNamespace, testClusterName, testStsName, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// PVC should remain
	remaining := &corev1.PersistentVolumeClaimList{}
	if err := c.List(ctx, remaining, client.InNamespace(testNamespace)); err != nil {
		t.Fatalf("listing PVCs: %v", err)
	}
	if len(remaining.Items) != 1 {
		t.Errorf("remaining PVCs = %d, want 1 (nil spec should be no-op)", len(remaining.Items))
	}
}

func TestDeleteCascadeDeletePVCs_AllCascadeFalse(t *testing.T) {
	spec := storageSpecWithCascade(false)

	c := buildFakeClient(
		newPVC("data-"+testStsName+"-0"),
		newPVC("data-"+testStsName+"-1"),
	)

	ctx := context.Background()
	err := DeleteCascadeDeletePVCs(ctx, c, testNamespace, testClusterName, testStsName, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	remaining := &corev1.PersistentVolumeClaimList{}
	if err := c.List(ctx, remaining, client.InNamespace(testNamespace)); err != nil {
		t.Fatalf("listing PVCs: %v", err)
	}
	if len(remaining.Items) != 2 {
		t.Errorf("remaining PVCs = %d, want 2 (no cascade volumes)", len(remaining.Items))
	}
}
