package controller

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

func newTestReconciler(scheme *runtime.Scheme, objs ...client.Object) (*AerospikeClusterReconciler, *record.FakeRecorder) {
	builder := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&ackov1alpha1.AerospikeCluster{}).
		WithObjects(objs...)

	recorder := record.NewFakeRecorder(16)
	return &AerospikeClusterReconciler{
		Client:   builder.Build(),
		Scheme:   scheme,
		Recorder: recorder,
	}, recorder
}

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	if err := ackov1alpha1.AddToScheme(s); err != nil {
		panic(err)
	}
	return s
}

func TestHandlePause_SetsPhaseAndCondition(t *testing.T) {
	scheme := testScheme()
	stored := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Status: ackov1alpha1.AerospikeClusterStatus{
			Phase: ackov1alpha1.AerospikePhaseCompleted,
		},
	}

	reconciler, recorder := newTestReconciler(scheme, stored)
	cluster := stored.DeepCopy()

	err := reconciler.HandlePause(context.Background(), cluster)
	if err != nil {
		t.Fatalf("HandlePause() error = %v", err)
	}

	// Verify Phase is Paused
	updated := &ackov1alpha1.AerospikeCluster{}
	if err := reconciler.Get(context.Background(), types.NamespacedName{Name: "demo", Namespace: "default"}, updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if updated.Status.Phase != ackov1alpha1.AerospikePhasePaused {
		t.Errorf("Phase = %q, want %q", updated.Status.Phase, ackov1alpha1.AerospikePhasePaused)
	}
	if updated.Status.PhaseReason != pausePhaseReason {
		t.Errorf("PhaseReason = %q, want %q", updated.Status.PhaseReason, pausePhaseReason)
	}

	// Verify ReconciliationPaused condition is True
	found := false
	for _, c := range updated.Status.Conditions {
		if c.Type == ackov1alpha1.ConditionReconciliationPaused {
			found = true
			if c.Status != metav1.ConditionTrue {
				t.Errorf("ReconciliationPaused condition status = %q, want True", c.Status)
			}
		}
	}
	if !found {
		t.Error("ReconciliationPaused condition not found")
	}

	// Verify caller object is updated
	if cluster.Status.Phase != ackov1alpha1.AerospikePhasePaused {
		t.Errorf("caller cluster Phase = %q, want %q", cluster.Status.Phase, ackov1alpha1.AerospikePhasePaused)
	}

	// Verify event was recorded
	select {
	case ev := <-recorder.Events:
		if ev == "" {
			t.Error("expected non-empty event")
		}
	default:
		t.Error("expected event to be recorded")
	}
}

func TestHandleResume_ClearsPausedCondition(t *testing.T) {
	scheme := testScheme()
	stored := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Status: ackov1alpha1.AerospikeClusterStatus{
			Phase:       ackov1alpha1.AerospikePhasePaused,
			PhaseReason: pausePhaseReason,
			Conditions: []metav1.Condition{
				{
					Type:               ackov1alpha1.ConditionReconciliationPaused,
					Status:             metav1.ConditionTrue,
					Reason:             "ReconciliationPaused",
					Message:            "Reconciliation is paused by user (spec.paused=true)",
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}

	reconciler, recorder := newTestReconciler(scheme, stored)
	cluster := stored.DeepCopy()

	err := reconciler.HandleResume(context.Background(), cluster)
	if err != nil {
		t.Fatalf("HandleResume() error = %v", err)
	}

	updated := &ackov1alpha1.AerospikeCluster{}
	if err := reconciler.Get(context.Background(), types.NamespacedName{Name: "demo", Namespace: "default"}, updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// Verify ReconciliationPaused condition is now False
	for _, c := range updated.Status.Conditions {
		if c.Type == ackov1alpha1.ConditionReconciliationPaused {
			if c.Status != metav1.ConditionFalse {
				t.Errorf("ReconciliationPaused condition status = %q, want False", c.Status)
			}
		}
	}

	// Verify event was recorded
	select {
	case ev := <-recorder.Events:
		if ev == "" {
			t.Error("expected non-empty event")
		}
	default:
		t.Error("expected event to be recorded for resume")
	}
}

func TestHandleResume_ClearsStaleErrorState(t *testing.T) {
	scheme := testScheme()
	stored := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Status: ackov1alpha1.AerospikeClusterStatus{
			Phase:                ackov1alpha1.AerospikePhasePaused,
			PhaseReason:          pausePhaseReason,
			FailedReconcileCount: 5,
			LastReconcileError:   "some error from before pause",
			Conditions: []metav1.Condition{
				{
					Type:               ackov1alpha1.ConditionReconciliationPaused,
					Status:             metav1.ConditionTrue,
					Reason:             "ReconciliationPaused",
					Message:            "Reconciliation is paused by user (spec.paused=true)",
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}

	reconciler, _ := newTestReconciler(scheme, stored)
	cluster := stored.DeepCopy()

	err := reconciler.HandleResume(context.Background(), cluster)
	if err != nil {
		t.Fatalf("HandleResume() error = %v", err)
	}

	updated := &ackov1alpha1.AerospikeCluster{}
	if err := reconciler.Get(context.Background(), types.NamespacedName{Name: "demo", Namespace: "default"}, updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if updated.Status.FailedReconcileCount != 0 {
		t.Errorf("FailedReconcileCount = %d, want 0", updated.Status.FailedReconcileCount)
	}
	if updated.Status.LastReconcileError != "" {
		t.Errorf("LastReconcileError = %q, want empty", updated.Status.LastReconcileError)
	}
}

func TestHandleResume_ClearsErrorPhaseState(t *testing.T) {
	scheme := testScheme()
	stored := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Status: ackov1alpha1.AerospikeClusterStatus{
			Phase:                ackov1alpha1.AerospikePhaseError,
			FailedReconcileCount: 3,
			LastReconcileError:   "some error",
		},
	}

	reconciler, _ := newTestReconciler(scheme, stored)
	cluster := stored.DeepCopy()

	err := reconciler.HandleResume(context.Background(), cluster)
	if err != nil {
		t.Fatalf("HandleResume() error = %v", err)
	}

	updated := &ackov1alpha1.AerospikeCluster{}
	if err := reconciler.Get(context.Background(), types.NamespacedName{Name: "demo", Namespace: "default"}, updated); err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if updated.Status.FailedReconcileCount != 0 {
		t.Errorf("FailedReconcileCount = %d, want 0", updated.Status.FailedReconcileCount)
	}
	if updated.Status.LastReconcileError != "" {
		t.Errorf("LastReconcileError = %q, want empty", updated.Status.LastReconcileError)
	}
}

func TestHandlePause_Idempotent(t *testing.T) {
	scheme := testScheme()
	stored := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Status: ackov1alpha1.AerospikeClusterStatus{
			Phase:       ackov1alpha1.AerospikePhasePaused,
			PhaseReason: pausePhaseReason,
			Conditions: []metav1.Condition{
				{
					Type:               ackov1alpha1.ConditionReconciliationPaused,
					Status:             metav1.ConditionTrue,
					Reason:             "ReconciliationPaused",
					Message:            "Reconciliation is paused by user (spec.paused=true)",
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}

	reconciler, _ := newTestReconciler(scheme, stored)
	cluster := stored.DeepCopy()

	// Calling HandlePause again should succeed without error
	err := reconciler.HandlePause(context.Background(), cluster)
	if err != nil {
		t.Fatalf("HandlePause() second call error = %v", err)
	}

	if cluster.Status.Phase != ackov1alpha1.AerospikePhasePaused {
		t.Errorf("Phase = %q, want %q", cluster.Status.Phase, ackov1alpha1.AerospikePhasePaused)
	}
}

func TestHandleResume_Idempotent(t *testing.T) {
	scheme := testScheme()
	stored := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo",
			Namespace: "default",
		},
		Status: ackov1alpha1.AerospikeClusterStatus{
			Phase: ackov1alpha1.AerospikePhaseCompleted,
			Conditions: []metav1.Condition{
				{
					Type:               ackov1alpha1.ConditionReconciliationPaused,
					Status:             metav1.ConditionFalse,
					Reason:             "ReconciliationResumed",
					Message:            "Reconciliation resumed",
					LastTransitionTime: metav1.Now(),
				},
			},
		},
	}

	reconciler, _ := newTestReconciler(scheme, stored)
	cluster := stored.DeepCopy()

	// Calling HandleResume on already-resumed cluster should succeed
	err := reconciler.HandleResume(context.Background(), cluster)
	if err != nil {
		t.Fatalf("HandleResume() on non-paused cluster error = %v", err)
	}
}
