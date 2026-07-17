package controller

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/intstr"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

// getScaleDownBatchSize is a method on AerospikeClusterReconciler but does not
// use the client — it only reads cluster.Spec fields and delegates to
// resolveIntOrPercent. We test it with a zero-value reconciler.

func TestGetScaleDownBatchSize_Default(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	cluster := &ackov1alpha1.AerospikeCluster{}

	// Default: scale down all at once
	got := r.getScaleDownBatchSize(cluster, 5)
	if got != 5 {
		t.Errorf("getScaleDownBatchSize default = %d, want 5 (total)", got)
	}
}

func TestGetScaleDownBatchSize_NilRackConfig(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			RackConfig: nil,
		},
	}

	got := r.getScaleDownBatchSize(cluster, 3)
	if got != 3 {
		t.Errorf("getScaleDownBatchSize with nil RackConfig = %d, want 3 (total)", got)
	}
}

func TestGetScaleDownBatchSize_RackConfigInt(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	batchSize := intstr.FromInt32(2)
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			RackConfig: &ackov1alpha1.RackConfig{
				ScaleDownBatchSize: &batchSize,
			},
		},
	}

	got := r.getScaleDownBatchSize(cluster, 5)
	if got != 2 {
		t.Errorf("getScaleDownBatchSize with int batch = %d, want 2", got)
	}
}

func TestGetScaleDownBatchSize_RackConfigPercent(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	batchSize := intstr.FromString("50%")
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			RackConfig: &ackov1alpha1.RackConfig{
				ScaleDownBatchSize: &batchSize,
			},
		},
	}

	// 50% of 6 = 3
	got := r.getScaleDownBatchSize(cluster, 6)
	if got != 3 {
		t.Errorf("getScaleDownBatchSize with 50%% of 6 = %d, want 3", got)
	}
}

func TestGetScaleDownBatchSize_NilScaleDownBatchSize(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			RackConfig: &ackov1alpha1.RackConfig{
				ScaleDownBatchSize: nil,
			},
		},
	}

	got := r.getScaleDownBatchSize(cluster, 4)
	if got != 4 {
		t.Errorf("getScaleDownBatchSize with nil ScaleDownBatchSize = %d, want 4 (total)", got)
	}
}

// getRollingUpdateBatchSize tests

func TestGetRollingUpdateBatchSize_Default(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	cluster := &ackov1alpha1.AerospikeCluster{}

	got := r.getRollingUpdateBatchSize(cluster, 5)
	if got != 1 {
		t.Errorf("getRollingUpdateBatchSize default = %d, want 1", got)
	}
}

func TestGetRollingUpdateBatchSize_SpecLevel(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	batchSize := int32(3)
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			RollingUpdateBatchSize: &batchSize,
		},
	}

	got := r.getRollingUpdateBatchSize(cluster, 10)
	if got != 3 {
		t.Errorf("getRollingUpdateBatchSize spec-level = %d, want 3", got)
	}
}

func TestGetRollingUpdateBatchSize_RackConfigTakesPrecedence(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	specBatch := int32(3)
	rackBatch := intstr.FromInt32(5)
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			RollingUpdateBatchSize: &specBatch,
			RackConfig: &ackov1alpha1.RackConfig{
				RollingUpdateBatchSize: &rackBatch,
			},
		},
	}

	got := r.getRollingUpdateBatchSize(cluster, 10)
	if got != 5 {
		t.Errorf("getRollingUpdateBatchSize RackConfig should take precedence = %d, want 5", got)
	}
}

func TestGetRollingUpdateBatchSize_RackConfigPercent(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	rackBatch := intstr.FromString("25%")
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			RackConfig: &ackov1alpha1.RackConfig{
				RollingUpdateBatchSize: &rackBatch,
			},
		},
	}

	// 25% of 8 = 2
	got := r.getRollingUpdateBatchSize(cluster, 8)
	if got != 2 {
		t.Errorf("getRollingUpdateBatchSize 25%% of 8 = %d, want 2", got)
	}
}

func TestGetRollingUpdateBatchSize_SpecLevelZero(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	batchSize := int32(0)
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			RollingUpdateBatchSize: &batchSize,
		},
	}

	// Zero is treated as not set, should default to 1
	got := r.getRollingUpdateBatchSize(cluster, 5)
	if got != 1 {
		t.Errorf("getRollingUpdateBatchSize with zero spec = %d, want 1 (default)", got)
	}
}

// getMaxIgnorablePods tests

func TestGetMaxIgnorablePods_Default(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	cluster := &ackov1alpha1.AerospikeCluster{}

	got := r.getMaxIgnorablePods(cluster, 5)
	if got != 0 {
		t.Errorf("getMaxIgnorablePods default = %d, want 0", got)
	}
}

func TestGetMaxIgnorablePods_RackConfigInt(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	maxIgnorable := intstr.FromInt32(2)
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			RackConfig: &ackov1alpha1.RackConfig{
				MaxIgnorablePods: &maxIgnorable,
			},
		},
	}

	got := r.getMaxIgnorablePods(cluster, 5)
	if got != 2 {
		t.Errorf("getMaxIgnorablePods with int = %d, want 2", got)
	}
}

func TestGetMaxIgnorablePods_RackConfigPercent(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	maxIgnorable := intstr.FromString("20%")
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			RackConfig: &ackov1alpha1.RackConfig{
				MaxIgnorablePods: &maxIgnorable,
			},
		},
	}

	// 20% of 10 = 2
	got := r.getMaxIgnorablePods(cluster, 10)
	if got != 2 {
		t.Errorf("getMaxIgnorablePods 20%% of 10 = %d, want 2", got)
	}
}

func TestGetMaxIgnorablePods_NilRackConfig(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			RackConfig: nil,
		},
	}

	got := r.getMaxIgnorablePods(cluster, 5)
	if got != 0 {
		t.Errorf("getMaxIgnorablePods with nil RackConfig = %d, want 0", got)
	}
}

func TestGetMaxIgnorablePods_NilMaxIgnorablePods(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			RackConfig: &ackov1alpha1.RackConfig{
				MaxIgnorablePods: nil,
			},
		},
	}

	got := r.getMaxIgnorablePods(cluster, 5)
	if got != 0 {
		t.Errorf("getMaxIgnorablePods with nil MaxIgnorablePods = %d, want 0", got)
	}
}

// TestGetMaxIgnorablePods_ExplicitZeroInt verifies that an explicit integer 0
// resolves to 0 ("ignore no unhealthy pods"). resolveIntOrPercent clamps to a
// minimum of 1, so without special handling an explicit 0 would silently become
// 1 and let the rolling restart skip one unhealthy pod the user told it not to.
func TestGetMaxIgnorablePods_ExplicitZeroInt(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	maxIgnorable := intstr.FromInt32(0)
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			RackConfig: &ackov1alpha1.RackConfig{
				MaxIgnorablePods: &maxIgnorable,
			},
		},
	}

	got := r.getMaxIgnorablePods(cluster, 5)
	if got != 0 {
		t.Errorf("getMaxIgnorablePods with explicit int 0 = %d, want 0", got)
	}
}

// TestGetMaxIgnorablePods_ExplicitZeroPercent verifies that an explicit "0%"
// resolves to 0 rather than being clamped up to 1.
func TestGetMaxIgnorablePods_ExplicitZeroPercent(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	maxIgnorable := intstr.FromString("0%")
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			RackConfig: &ackov1alpha1.RackConfig{
				MaxIgnorablePods: &maxIgnorable,
			},
		},
	}

	got := r.getMaxIgnorablePods(cluster, 10)
	if got != 0 {
		t.Errorf("getMaxIgnorablePods with explicit \"0%%\" = %d, want 0", got)
	}
}

// TestGetMaxIgnorablePods_PercentRoundsDownToZero verifies that a non-zero
// percentage that rounds down to zero (e.g. "10%" of a 3-pod cluster) is NOT
// treated as an explicit zero: resolveIntOrPercent's minimum-of-1 clamp still
// applies so the user's intent ("ignore a small fraction") yields at least 1.
func TestGetMaxIgnorablePods_PercentRoundsDownToZero(t *testing.T) {
	r := &AerospikeClusterReconciler{}
	maxIgnorable := intstr.FromString("10%")
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{
			RackConfig: &ackov1alpha1.RackConfig{
				MaxIgnorablePods: &maxIgnorable,
			},
		},
	}

	got := r.getMaxIgnorablePods(cluster, 3)
	if got != 1 {
		t.Errorf("getMaxIgnorablePods with \"10%%\" of 3 pods = %d, want 1", got)
	}
}
