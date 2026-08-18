//go:build integration

package controller

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

// --- the scale subresource enforces the CE size rules ---
//
// #353 reported that `kubectl scale` bypasses admission, because the webhook
// rules list `aerospikeclusters` and not `aerospikeclusters/scale`. That part is
// true, but the suggested fix — adding the subresource to the rules — does not
// work: for a CustomResource the apiserver hands a validating webhook an
// `autoscaling/v1 Scale`, not the AerospikeCluster
// (apiextensions-apiserver/pkg/apiserver/customresource_handler.go:1021 sets
// scaleScope.Kind to Scale, and pkg/registry/customresource/etcd.go:230-241
// converts the CR to a Scale before admission validation runs). The typed
// validator would decode that Scale into an AerospikeCluster with size 0 and
// reject every scale request under failurePolicy: Fail.
//
// So the two size rules are enforced in the CRD schema instead, where validation
// does apply to scale writes. These specs run against a real apiserver and pin
// that, on the scale path specifically:
//
//   - size above the CE maximum is rejected (already true; pinned so it stays)
//   - size 0 is rejected     (was accepted: full cluster outage)
//   - racks > size is rejected (was accepted: racks driven to 0 replicas)
//
// The last two fail without the schema rules added in this change.

const (
	scaleTestNamespace = "default"
	scaleTestImage     = "aerospike:ce-8.1.1.1"
)

// scaleTestCluster builds a minimal valid cluster. The webhook is not running in
// envtest, so this must already satisfy the CRD schema on its own.
func scaleTestCluster(name string, size int32, racks []ackov1alpha1.Rack) *ackov1alpha1.AerospikeCluster {
	cluster := &ackov1alpha1.AerospikeCluster{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: scaleTestNamespace},
		Spec: ackov1alpha1.AerospikeClusterSpec{
			Size:  size,
			Image: scaleTestImage,
		},
	}
	if len(racks) > 0 {
		cluster.Spec.RackConfig = &ackov1alpha1.RackConfig{Racks: racks}
	}
	return cluster
}

// scaleTo performs the same write `kubectl scale` performs: an update against
// the /scale subresource, not against the main resource.
func scaleTo(cluster *ackov1alpha1.AerospikeCluster, replicas int32) error {
	scale := &autoscalingv1.Scale{
		ObjectMeta: metav1.ObjectMeta{Name: cluster.Name, Namespace: cluster.Namespace},
		Spec:       autoscalingv1.ScaleSpec{Replicas: replicas},
	}
	return k8sClient.SubResource("scale").Update(ctx, cluster, client.WithSubResourceBody(scale))
}

var _ = Describe("AerospikeCluster scale subresource", func() {
	var cluster *ackov1alpha1.AerospikeCluster

	AfterEach(func() {
		if cluster != nil {
			_ = k8sClient.Delete(ctx, cluster)
			cluster = nil
		}
	})

	It("rejects a scale above the CE maximum", func() {
		cluster = scaleTestCluster("scale-max", 3, nil)
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())

		err := scaleTo(cluster, 99)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("should be less than or equal to 8"))
	})

	It("rejects scaling to zero", func() {
		cluster = scaleTestCluster("scale-zero", 3, nil)
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())

		err := scaleTo(cluster, 0)
		Expect(err).To(HaveOccurred(), "scaling to 0 takes the whole cluster down and must be refused")
		Expect(err.Error()).To(ContainSubstring("spec.size"))

		// The stored object must be untouched, not merely reported as failed.
		stored := &ackov1alpha1.AerospikeCluster{}
		Expect(k8sClient.Get(ctx,
			types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, stored)).To(Succeed())
		Expect(stored.Spec.Size).To(Equal(int32(3)))
	})

	It("rejects a scale that leaves fewer pods than racks", func() {
		racks := []ackov1alpha1.Rack{{ID: 1}, {ID: 2}, {ID: 3}}
		cluster = scaleTestCluster("scale-racks", 3, racks)
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())

		err := scaleTo(cluster, 1)
		Expect(err).To(HaveOccurred(),
			"1 pod across 3 racks gives two racks a 0-replica StatefulSet")
		Expect(err.Error()).To(ContainSubstring("rack"))

		stored := &ackov1alpha1.AerospikeCluster{}
		Expect(k8sClient.Get(ctx,
			types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, stored)).To(Succeed())
		Expect(stored.Spec.Size).To(Equal(int32(3)))
	})

	It("still allows a legitimate scale", func() {
		cluster = scaleTestCluster("scale-ok", 3, nil)
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())

		Expect(scaleTo(cluster, 5)).To(Succeed(), fmt.Sprintf("scaling %s to 5 is within CE limits", cluster.Name))

		stored := &ackov1alpha1.AerospikeCluster{}
		Expect(k8sClient.Get(ctx,
			types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, stored)).To(Succeed())
		Expect(stored.Spec.Size).To(Equal(int32(5)))
	})

	It("still allows a scale down to a size that keeps one pod per rack", func() {
		racks := []ackov1alpha1.Rack{{ID: 1}, {ID: 2}}
		cluster = scaleTestCluster("scale-racks-ok", 6, racks)
		Expect(k8sClient.Create(ctx, cluster)).To(Succeed())

		Expect(scaleTo(cluster, 2)).To(Succeed())

		stored := &ackov1alpha1.AerospikeCluster{}
		Expect(k8sClient.Get(ctx,
			types.NamespacedName{Name: cluster.Name, Namespace: cluster.Namespace}, stored)).To(Succeed())
		Expect(stored.Spec.Size).To(Equal(int32(2)))
	})
})
