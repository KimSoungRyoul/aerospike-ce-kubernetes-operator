package controller

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

// TestMarkDirtyVolumes_PreservesConcurrentPodStatus is the regression test for
// the markDirtyVolumes fix. markDirtyVolumes must use a MergeFrom status patch,
// not a full Status().Update — a full replace clobbers Status.Pods fields that
// other reconcilers (updateDynamicConfigStatus / recordPodRestartStatus) write
// concurrently with their own MergeFrom patches.
//
// The test interposes a concurrent writer between markDirtyVolumes' refetch and
// its status write: the interceptor stamps DynamicConfigStatus on a *different*
// pod just before the write is delegated to the real client. With the buggy
// full Update, markDirtyVolumes' write is built from a status snapshot taken
// before that concurrent write, so it overwrites the whole status and the
// concurrent field is lost. With the MergeFrom patch fix, only the target pod's
// DirtyVolumes field is sent, so the concurrent field survives.
func TestMarkDirtyVolumes_PreservesConcurrentPodStatus(t *testing.T) {
	scheme := rollingRestartScheme(t)

	const (
		clusterName = "demo"
		namespace   = "default"
		targetPod   = "demo-0" // markDirtyVolumes writes DirtyVolumes here
		otherPod    = "demo-1" // a concurrent reconciler writes DynamicConfigStatus here
	)

	cluster := &ackov1alpha1.AerospikeCluster{}
	cluster.Name = clusterName
	cluster.Namespace = namespace
	cluster.Status.Pods = map[string]ackov1alpha1.AerospikePodStatus{
		targetPod: {},
		otherPod:  {},
	}

	key := types.NamespacedName{Name: clusterName, Namespace: namespace}

	// concurrentWrite simulates another reconciler patching otherPod's
	// DynamicConfigStatus. It runs exactly once, just before markDirtyVolumes'
	// own status write is delegated to the underlying fake client.
	var base client.WithWatch
	concurrentDone := false
	concurrentWrite := func(ctx context.Context) {
		if concurrentDone {
			return
		}
		concurrentDone = true
		latest := &ackov1alpha1.AerospikeCluster{}
		if err := base.Get(ctx, key, latest); err != nil {
			t.Fatalf("concurrent writer: Get error = %v", err)
		}
		patch := client.MergeFrom(latest.DeepCopy())
		ps := latest.Status.Pods[otherPod]
		ps.DynamicConfigStatus = "Applied"
		latest.Status.Pods[otherPod] = ps
		if err := base.Status().Patch(ctx, latest, patch); err != nil {
			t.Fatalf("concurrent writer: Patch error = %v", err)
		}
	}

	base = fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&ackov1alpha1.AerospikeCluster{}).
		WithObjects(cluster).
		Build()

	wrapped := interceptor.NewClient(base, interceptor.Funcs{
		SubResourceUpdate: func(ctx context.Context, c client.Client, sub string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
			concurrentWrite(ctx)
			return c.SubResource(sub).Update(ctx, obj, opts...)
		},
		SubResourcePatch: func(ctx context.Context, c client.Client, sub string, obj client.Object, p client.Patch, opts ...client.SubResourcePatchOption) error {
			concurrentWrite(ctx)
			return c.SubResource(sub).Patch(ctx, obj, p, opts...)
		},
	})

	reconciler := &AerospikeClusterReconciler{
		Client:   wrapped,
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(8),
	}

	if err := reconciler.markDirtyVolumes(context.Background(), cluster, targetPod, []string{"data"}); err != nil {
		t.Fatalf("markDirtyVolumes() error = %v", err)
	}

	got := &ackov1alpha1.AerospikeCluster{}
	if err := base.Get(context.Background(), key, got); err != nil {
		t.Fatalf("Get final cluster error = %v", err)
	}

	// markDirtyVolumes must have written DirtyVolumes on the target pod.
	if dv := got.Status.Pods[targetPod].DirtyVolumes; len(dv) != 1 || dv[0] != "data" {
		t.Errorf("targetPod DirtyVolumes = %v, want [data]", dv)
	}

	// The concurrent writer's DynamicConfigStatus on otherPod must survive.
	// A full Status().Update would clobber it back to "".
	if got.Status.Pods[otherPod].DynamicConfigStatus != "Applied" {
		t.Errorf("otherPod DynamicConfigStatus = %q, want %q — markDirtyVolumes clobbered a concurrent status write",
			got.Status.Pods[otherPod].DynamicConfigStatus, "Applied")
	}
}
