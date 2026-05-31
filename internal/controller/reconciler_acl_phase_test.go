package controller

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

// TestTerminalPhaseForACL verifies the phase/reason decision made at the end of
// reconcileCluster. The regression this guards: an ACL sync failure used to be
// recorded only as the ACLSynced=False condition while the cluster phase was
// still hard-coded to Completed/"healthy and stable" — a silent failure that
// misled any consumer keying on phase=Completed.
func TestTerminalPhaseForACL(t *testing.T) {
	tests := []struct {
		name       string
		aclErr     error
		wantPhase  ackov1alpha1.AerospikePhase
		wantReason string
	}{
		{
			name:       "acl synced -> Completed/healthy",
			aclErr:     nil,
			wantPhase:  ackov1alpha1.AerospikePhaseCompleted,
			wantReason: "Cluster is healthy and stable",
		},
		{
			name:       "acl failed -> ACLSync phase, not Completed",
			aclErr:     errors.New("granting roles to user admin: connection reset"),
			wantPhase:  ackov1alpha1.AerospikePhaseACLSync,
			wantReason: "ACL synchronization failed; will retry: granting roles to user admin: connection reset",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPhase, gotReason := terminalPhaseForACL(tt.aclErr)
			if gotPhase != tt.wantPhase {
				t.Errorf("phase = %q, want %q", gotPhase, tt.wantPhase)
			}
			if gotReason != tt.wantReason {
				t.Errorf("reason = %q, want %q", gotReason, tt.wantReason)
			}
			// A failed ACL sync must never be reported as the healthy Completed
			// phase — that is the precise bug being fixed.
			if tt.aclErr != nil && gotPhase == ackov1alpha1.AerospikePhaseCompleted {
				t.Errorf("ACL failure must not map to Completed phase")
			}
		})
	}
}

// TestUpdateStatusAndPhaseACLFailureDoesNotStampAppliedSpec is the integration
// half of the fix: when reconcileCluster publishes the ACLSync phase (because
// ACL failed), updateStatusAndPhase must NOT record Status.AppliedSpec — doing
// so would treat the unsynced spec as the drift-detection baseline. AppliedSpec
// is only stamped on the Completed phase, so driving the cluster through the
// ACLSync phase must leave AppliedSpec nil.
func TestUpdateStatusAndPhaseACLFailureDoesNotStampAppliedSpec(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := ackov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("corev1 AddToScheme() error = %v", err)
	}

	stored := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size:  2,
			Image: "aerospike:ce-8.1.1.1",
			AerospikeAccessControl: &ackov1alpha1.AerospikeAccessControlSpec{
				Users: []ackov1alpha1.AerospikeUserSpec{
					{Name: "admin", SecretName: "admin-secret", Roles: []string{"sys-admin", "user-admin"}},
				},
			},
		},
		Status: ackov1alpha1.AerospikeClusterStatus{
			Phase:       ackov1alpha1.AerospikePhaseACLSync,
			PhaseReason: "Synchronizing ACL roles and users",
		},
	}

	reconciler := &AerospikeClusterReconciler{
		Client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&ackov1alpha1.AerospikeCluster{}).
			WithObjects(stored).
			Build(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(10),
	}

	aclErr := errors.New("changing password for user admin: timeout")
	phase, phaseReason := terminalPhaseForACL(aclErr)

	nn := types.NamespacedName{Name: stored.Name, Namespace: stored.Namespace}
	opts := StatusUpdateOpts{ACLErr: aclErr, ACLSynced: false}
	if err := reconciler.updateStatusAndPhase(context.Background(), nn, phase, phaseReason, opts); err != nil {
		t.Fatalf("updateStatusAndPhase() error = %v", err)
	}

	updated := &ackov1alpha1.AerospikeCluster{}
	if err := reconciler.Get(context.Background(), nn, updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if updated.Status.Phase != ackov1alpha1.AerospikePhaseACLSync {
		t.Errorf("Phase = %q, want %q (ACL failure must not be reported as Completed)",
			updated.Status.Phase, ackov1alpha1.AerospikePhaseACLSync)
	}
	if updated.Status.AppliedSpec != nil {
		t.Errorf("AppliedSpec must remain nil on ACL failure; got %+v", updated.Status.AppliedSpec)
	}

	// The ACLSynced condition must carry the detailed failure.
	var aclCond *metav1.Condition
	for i := range updated.Status.Conditions {
		if updated.Status.Conditions[i].Type == ackov1alpha1.ConditionACLSynced {
			aclCond = &updated.Status.Conditions[i]
			break
		}
	}
	if aclCond == nil {
		t.Fatal("expected ACLSynced condition to be set")
	}
	if aclCond.Status != metav1.ConditionFalse {
		t.Errorf("ACLSynced condition status = %q, want False", aclCond.Status)
	}
}
