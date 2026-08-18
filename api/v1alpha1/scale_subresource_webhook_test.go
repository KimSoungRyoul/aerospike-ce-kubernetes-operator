package v1alpha1

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestValidatorCannotValidateAScaleObject records why #353's suggested fix —
// adding `aerospikeclusters/scale` to the webhook rules — was NOT applied.
//
// For a CustomResource the apiserver does not hand the webhook the custom
// resource on a scale request. apiextensions-apiserver sets the scale endpoint's
// kind to autoscaling/v1 Scale (pkg/apiserver/customresource_handler.go:1021)
// and converts the CR to a Scale before admission validation runs
// (pkg/registry/customresource/etcd.go:230-241, toScaleUpdateValidation).
//
// This test feeds the validator exactly what it would receive in that case: a
// Scale payload decoded into an AerospikeCluster. The result is a cluster with
// size 0 and no image, which the validator rejects. With failurePolicy: Fail on
// the rules, registering the subresource would therefore refuse EVERY
// `kubectl scale`, including legitimate ones — worse than the bypass it was
// meant to close.
//
// The two size rules are enforced in the CRD schema instead, where validation
// does run on scale writes. TestControllers'
// "AerospikeCluster scale subresource" specs pin that against a real apiserver.
func TestValidatorCannotValidateAScaleObject(t *testing.T) {
	// The body the apiserver would send for `kubectl scale asc/demo --replicas=5`.
	scalePayload := []byte(`{
		"kind": "Scale",
		"apiVersion": "autoscaling/v1",
		"metadata": {"name": "demo", "namespace": "aerospike"},
		"spec": {"replicas": 5},
		"status": {"replicas": 3, "selector": "app.kubernetes.io/name=aerospike"}
	}`)

	cluster := &AerospikeCluster{}
	if err := json.Unmarshal(scalePayload, cluster); err != nil {
		t.Fatalf("decoding a Scale into an AerospikeCluster: %v", err)
	}

	// A Scale carries spec.replicas, not spec.size, so the replica count the user
	// asked for is simply not visible to a typed AerospikeCluster validator.
	if cluster.Spec.Size != 0 {
		t.Fatalf("Spec.Size = %d, want 0: a Scale has no spec.size for the decoder to read", cluster.Spec.Size)
	}

	v := &AerospikeClusterValidator{}
	_, err := v.validate(cluster)
	if err == nil {
		t.Fatal("validate() returned no error; expected the decoded Scale to be rejected")
	}
	if !strings.Contains(err.Error(), "spec.size") {
		t.Errorf("validate() error = %q, want it to mention spec.size", err.Error())
	}
}
