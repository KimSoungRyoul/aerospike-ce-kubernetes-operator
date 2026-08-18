package controller

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/utils"
)

// --- upgrading the operator must not restart a fleet ---
//
// #340 folded spec.storage into computePodSpecHash. That changed the pod-spec
// hash of EVERY cluster carrying any spec.storage, with no user edit at all, so
// installing the new operator queued every pod of every cluster for a cold
// restart — each with a full data migration, and during exactly the window #341
// exists to keep pods from being deleted in.
//
// Storage now lives in its own annotation, so the pod-spec hash is unchanged
// across the upgrade and a pod that predates the storage annotation is treated
// as matching rather than stale.

func upgradeTestStorage(path string) *ackov1alpha1.AerospikeStorageSpec {
	return &ackov1alpha1.AerospikeStorageSpec{
		Volumes: []ackov1alpha1.VolumeSpec{{
			Name:      "data",
			Source:    ackov1alpha1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			Aerospike: &ackov1alpha1.AerospikeVolumeAttachment{Path: path},
		}},
	}
}

func upgradeTestCluster(storage *ackov1alpha1.AerospikeStorageSpec) *ackov1alpha1.AerospikeCluster {
	return &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: ctrlTestNamespace},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size:    3,
			Image:   "aerospike:ce-8.1.1.1",
			Storage: storage,
		},
	}
}

// TestComputePodSpecHash_UnchangedByStorage is the direct regression: the
// pod-spec hash of a cluster WITH storage must equal that of the same cluster
// WITHOUT it, because storage is not part of that hash. When storage was folded
// in, these differed and every existing pod's annotation went stale at once.
func TestComputePodSpecHash_UnchangedByStorage(t *testing.T) {
	rack := &ackov1alpha1.Rack{ID: 0}

	withStorage := computePodSpecHash(upgradeTestCluster(upgradeTestStorage("/opt/aerospike/data")), rack)
	noStorage := computePodSpecHash(upgradeTestCluster(nil), rack)

	if withStorage != noStorage {
		t.Fatalf("pod-spec hash differs with and without storage (%q vs %q); "+
			"storage must not be part of this hash, or upgrading the operator "+
			"invalidates every existing pod's annotation and restarts the fleet",
			withStorage, noStorage)
	}

	// A storage edit must still not move the pod-spec hash — only the storage hash.
	moved := computePodSpecHash(upgradeTestCluster(upgradeTestStorage("/opt/aerospike/moved")), rack)
	if moved != withStorage {
		t.Errorf("pod-spec hash changed on a storage edit: %q vs %q", moved, withStorage)
	}
	if computeStorageHash(upgradeTestCluster(upgradeTestStorage("/opt/aerospike/moved")), rack) ==
		computeStorageHash(upgradeTestCluster(upgradeTestStorage("/opt/aerospike/data")), rack) {
		t.Error("storage hash did not change on a storage edit; the #340 guarantee was lost")
	}
}

// upgradePod builds a pod as an older operator would have left it: config and
// pod-spec annotations present, NO storage annotation.
func upgradePod(name, configHash, podSpecHash, storageHash string) corev1.Pod {
	annotations := map[string]string{
		utils.ConfigHashAnnotation:  configHash,
		utils.PodSpecHashAnnotation: podSpecHash,
	}
	if storageHash != "" {
		annotations[utils.StorageHashAnnotation] = storageHash
	}
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   ctrlTestNamespace,
			Labels:      utils.LabelsForRack("demo", 0),
			Annotations: annotations,
		},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
		},
	}
}

// TestSelectPodsToRestart_StorageAnnotationGrandfathering is the test the review
// asked for: an existing StatefulSet carrying a pre-upgrade hash must not queue
// any pod for restart.
func TestSelectPodsToRestart_StorageAnnotationGrandfathering(t *testing.T) {
	tests := []struct {
		name string
		// what the pod carries
		podConfigHash, podPodSpecHash, podStorageHash string
		// what the template wants
		wantConfigHash, wantPodSpecHash, wantStorageHash string
		wantRestart                                      bool
	}{
		{
			// The upgrade case. Everything the old operator knew about matches;
			// the storage annotation simply did not exist yet.
			name:          "pod predating the storage annotation is not restarted",
			podConfigHash: "cfg", podPodSpecHash: "spec", podStorageHash: "",
			wantConfigHash: "cfg", wantPodSpecHash: "spec", wantStorageHash: "storage-A",
			wantRestart: false,
		},
		{
			name:          "pod carrying a matching storage hash is not restarted",
			podConfigHash: "cfg", podPodSpecHash: "spec", podStorageHash: "storage-A",
			wantConfigHash: "cfg", wantPodSpecHash: "spec", wantStorageHash: "storage-A",
			wantRestart: false,
		},
		{
			// Once a pod carries the annotation, a real storage edit must roll it —
			// this is the #340 guarantee, preserved.
			name:          "pod carrying a stale storage hash is restarted",
			podConfigHash: "cfg", podPodSpecHash: "spec", podStorageHash: "storage-A",
			wantConfigHash: "cfg", wantPodSpecHash: "spec", wantStorageHash: "storage-B",
			wantRestart: true,
		},
		{
			// Grandfathering must not swallow the other reasons.
			name:          "config change still restarts a pod with no storage annotation",
			podConfigHash: "cfg-old", podPodSpecHash: "spec", podStorageHash: "",
			wantConfigHash: "cfg-new", wantPodSpecHash: "spec", wantStorageHash: "storage-A",
			wantRestart: true,
		},
		{
			name:          "pod-spec change still restarts a pod with no storage annotation",
			podConfigHash: "cfg", podPodSpecHash: "spec-old", podStorageHash: "",
			wantConfigHash: "cfg", wantPodSpecHash: "spec-new", wantStorageHash: "storage-A",
			wantRestart: true,
		},
	}

	scheme := runtime.NewScheme()
	if err := ackov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("acko AddToScheme() error = %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("corev1 AddToScheme() error = %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := upgradeTestCluster(upgradeTestStorage("/opt/aerospike/data"))
			r := &AerospikeClusterReconciler{
				Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build(),
				Scheme:   scheme,
				Recorder: record.NewFakeRecorder(8),
			}
			pod := upgradePod("demo-0-0", tt.podConfigHash, tt.podPodSpecHash, tt.podStorageHash)

			selected, _ := r.selectPodsToRestart(context.Background(), cluster, []corev1.Pod{pod},
				tt.wantConfigHash, tt.wantPodSpecHash, tt.wantStorageHash, 0)

			if got := len(selected) > 0; got != tt.wantRestart {
				t.Errorf("pod queued for restart = %v, want %v", got, tt.wantRestart)
			}
		})
	}
}

// TestReconcileStatefulSet_UpgradeAddsStorageAnnotationWithoutRestarting pins the
// whole path: a StatefulSet templated by an older operator (no storage
// annotation, pod-spec hash computed the old way) gets the annotation added, and
// the pod-spec hash it already carries stays valid — so nothing is queued.
func TestReconcileStatefulSet_UpgradeAddsStorageAnnotationWithoutRestarting(t *testing.T) {
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		ackov1alpha1.AddToScheme, corev1.AddToScheme, appsv1.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			t.Fatalf("AddToScheme() error = %v", err)
		}
	}

	cluster := upgradeTestCluster(upgradeTestStorage("/opt/aerospike/data"))
	rack := &ackov1alpha1.Rack{ID: 0}

	// The hash an older operator would have stamped: today's formula, which this
	// change restored to its pre-#340 shape.
	preUpgradePodSpecHash := computePodSpecHash(cluster, rack)

	replicas := int32(3)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      utils.StatefulSetName(cluster.Name, rack.ID),
			Namespace: ctrlTestNamespace,
			Labels:    utils.LabelsForCluster(cluster.Name),
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						utils.ConfigHashAnnotation:  "cfg",
						utils.PodSpecHashAnnotation: preUpgradePodSpecHash,
						// No storage annotation: this StatefulSet predates it.
					},
				},
			},
		},
	}

	r := &AerospikeClusterReconciler{
		Client: fake.NewClientBuilder().WithScheme(scheme).
			WithStatusSubresource(&ackov1alpha1.AerospikeCluster{}).
			WithObjects(cluster, sts).Build(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(16),
	}

	if _, err := r.reconcileStatefulSet(context.Background(), cluster, rack, nil, "cfg", replicas); err != nil {
		t.Fatalf("reconcileStatefulSet() error = %v", err)
	}

	updated := &appsv1.StatefulSet{}
	if err := r.Get(context.Background(),
		types.NamespacedName{Name: sts.Name, Namespace: ctrlTestNamespace}, updated); err != nil {
		t.Fatalf("Get StatefulSet: %v", err)
	}

	// The pod-spec hash the running pods carry must be untouched, or every pod
	// goes stale at once.
	if got := updated.Spec.Template.Annotations[utils.PodSpecHashAnnotation]; got != preUpgradePodSpecHash {
		t.Errorf("pod-spec hash changed across the upgrade: %q -> %q; every existing pod would be queued",
			preUpgradePodSpecHash, got)
	}
	// And the storage annotation is now present, so later storage edits roll pods.
	if got := updated.Spec.Template.Annotations[utils.StorageHashAnnotation]; got == "" {
		t.Error("storage annotation was not added to the template")
	} else if want := computeStorageHash(cluster, rack); got != want {
		t.Errorf("storage annotation = %q, want %q", got, want)
	}
}

// TestSelectPodsToRestart_AdoptsStorageAnnotationWithoutRestarting closes the
// blind spot review found: a pre-upgrade pod carries no storage annotation, so it
// is never selected for a storage edit — and nothing bounds how long that lasts.
// A stable cluster with no config or image change keeps those pods for months, so
// #340's guarantee would not hold for the entire pre-upgrade fleet.
//
// The pod is now stamped with the desired hash, WITHOUT a restart, the first time
// it is seen matching on everything else.
func TestSelectPodsToRestart_AdoptsStorageAnnotationWithoutRestarting(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := ackov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("acko AddToScheme() error = %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("corev1 AddToScheme() error = %v", err)
	}

	cluster := upgradeTestCluster(upgradeTestStorage("/opt/aerospike/data"))
	pod := upgradePod("demo-0-0", "cfg", "spec", "")
	r := &AerospikeClusterReconciler{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, &pod).Build(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(8),
	}

	selected, _ := r.selectPodsToRestart(context.Background(), cluster, []corev1.Pod{pod},
		"cfg", "spec", "storage-A", 0)
	if len(selected) != 0 {
		t.Fatalf("pod queued for restart = %d, want 0; adoption must not cost a restart", len(selected))
	}

	live := &corev1.Pod{}
	if err := r.Get(context.Background(),
		types.NamespacedName{Name: pod.Name, Namespace: ctrlTestNamespace}, live); err != nil {
		t.Fatalf("Get pod: %v", err)
	}
	if got := live.Annotations[utils.StorageHashAnnotation]; got != "storage-A" {
		t.Fatalf("storage annotation = %q, want %q; without it this pod would never roll for a storage edit",
			got, "storage-A")
	}

	// Having been adopted, the very next storage edit must now roll it — this is
	// the guarantee the blind spot was swallowing.
	selected, _ = r.selectPodsToRestart(context.Background(), cluster, []corev1.Pod{*live},
		"cfg", "spec", "storage-B", 0)
	if len(selected) != 1 {
		t.Errorf("adopted pod queued for a storage edit = %d, want 1", len(selected))
	}
}

// TestSelectPodsToRestart_DoesNotAdoptWhenAlreadyRestarting pins that adoption is
// skipped for a pod being restarted anyway: the replacement picks the annotation
// up from the template, so patching the doomed pod is a pointless write.
func TestSelectPodsToRestart_DoesNotAdoptWhenAlreadyRestarting(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := ackov1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("acko AddToScheme() error = %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("corev1 AddToScheme() error = %v", err)
	}

	cluster := upgradeTestCluster(upgradeTestStorage("/opt/aerospike/data"))
	pod := upgradePod("demo-0-0", "cfg-old", "spec", "")
	r := &AerospikeClusterReconciler{
		Client:   fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, &pod).Build(),
		Scheme:   scheme,
		Recorder: record.NewFakeRecorder(8),
	}

	selected, _ := r.selectPodsToRestart(context.Background(), cluster, []corev1.Pod{pod},
		"cfg-new", "spec", "storage-A", 0)
	if len(selected) != 1 {
		t.Fatalf("pod queued for restart = %d, want 1 (config changed)", len(selected))
	}

	live := &corev1.Pod{}
	if err := r.Get(context.Background(),
		types.NamespacedName{Name: pod.Name, Namespace: ctrlTestNamespace}, live); err != nil {
		t.Fatalf("Get pod: %v", err)
	}
	if _, stamped := live.Annotations[utils.StorageHashAnnotation]; stamped {
		t.Error("a pod already being restarted was patched; its replacement gets the annotation from the template")
	}
}
