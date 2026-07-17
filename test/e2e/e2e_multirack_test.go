//go:build e2e

/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/test/utils"
)

const multirackNS = "aerospike-multirack"

var _ = Describe("Multi-rack cluster", Ordered, Label("heavy"), func() {

	BeforeAll(func() {
		By("creating multirack test namespace")
		Expect(utils.EnsureNamespace(ctx, k8sClient, multirackNS)).To(Succeed())
	})

	AfterAll(func() {
		By("cleaning up all multirack test clusters")
		for _, name := range []string{"e2e-mr-basic", "e2e-mr-scale"} {
			cmd := exec.Command("kubectl", "delete", "aerospikecluster", name,
				"-n", multirackNS, "--ignore-not-found", "--timeout=120s")
			_, _ = utils.Run(cmd)
		}
		By("deleting multirack test namespace")
		_ = utils.DeleteNamespace(ctx, k8sClient, multirackNS)
	})

	Context("Basic multi-rack deployment", func() {
		const clusterName = "e2e-mr-basic"

		It("should create a multi-rack cluster with correct StatefulSets", func() {
			By("creating a 4-node cluster with 2 racks")
			cluster := newTestCluster(clusterName, multirackNS, 4, func(c *ackov1alpha1.AerospikeCluster) {
				c.Spec.RackConfig = &ackov1alpha1.RackConfig{
					Racks: []ackov1alpha1.Rack{
						{ID: 1},
						{ID: 2},
					},
				}
			})
			Eventually(func() error {
				return k8sClient.Create(ctx, cluster)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("waiting for Completed phase")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, multirackNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())

			By("verifying one StatefulSet per rack is created")
			Eventually(func(g Gomega) {
				stsList, err := utils.ListClusterStatefulSets(ctx, k8sClient, clusterName, multirackNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(stsList.Items).To(HaveLen(2))

				stsNames := make([]string, 0, len(stsList.Items))
				for _, sts := range stsList.Items {
					stsNames = append(stsNames, sts.Name)
				}
				g.Expect(stsNames).To(ContainElements(
					fmt.Sprintf("%s-1", clusterName),
					fmt.Sprintf("%s-2", clusterName),
				))
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("verifying total pod count matches spec.size")
			Eventually(func(g Gomega) {
				count, err := utils.CountReadyPods(ctx, k8sClient, clusterName, multirackNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(count).To(Equal(4))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())

			By("verifying pods are distributed evenly across racks")
			Eventually(func(g Gomega) {
				podList, err := utils.ListClusterPods(ctx, k8sClient, clusterName, multirackNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(podList.Items).To(HaveLen(4))

				rackCounts := map[string]int{}
				for _, pod := range podList.Items {
					rack := pod.Labels["acko.io/rack"]
					g.Expect(rack).NotTo(BeEmpty(), "pod %s should have rack label", pod.Name)
					rackCounts[rack]++
				}
				g.Expect(rackCounts).To(HaveLen(2))
				for rack, count := range rackCounts {
					g.Expect(count).To(Equal(2), "rack %s should have 2 pods", rack)
				}
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("verifying one ConfigMap per rack is created")
			Eventually(func(g Gomega) {
				for _, rackID := range []int{1, 2} {
					cmName := fmt.Sprintf("%s-%d-config", clusterName, rackID)
					exists, err := utils.ConfigMapExists(ctx, k8sClient, cmName, multirackNS)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(exists).To(BeTrue(), "configmap %s should exist", cmName)
				}
			}, defaultTimeout, 2*time.Second).Should(Succeed())
		})

		It("should report correct status for multi-rack cluster", func() {
			Eventually(func(g Gomega) {
				cluster, err := utils.GetCluster(ctx, k8sClient, clusterName, multirackNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(cluster.Status.Size).To(Equal(int32(4)))
				g.Expect(cluster.Status.Pods).To(HaveLen(4))

				for _, ps := range cluster.Status.Pods {
					g.Expect(ps.IsRunningAndReady).To(BeTrue())
					g.Expect(ps.Image).To(Equal("aerospike:ce-8.1.1.1"))
					g.Expect(ps.Rack).To(BeElementOf(1, 2))
				}
			}, defaultTimeout, 2*time.Second).Should(Succeed())
		})
	})

	Context("Multi-rack scale up", func() {
		const clusterName = "e2e-mr-scale"

		It("should scale up and distribute pods across racks", func() {
			By("creating a 2-node cluster with 2 racks")
			cluster := newTestCluster(clusterName, multirackNS, 2, func(c *ackov1alpha1.AerospikeCluster) {
				c.Spec.RackConfig = &ackov1alpha1.RackConfig{
					Racks: []ackov1alpha1.Rack{
						{ID: 1},
						{ID: 2},
					},
				}
			})
			Eventually(func() error {
				return k8sClient.Create(ctx, cluster)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("waiting for Completed phase with 2 pods")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, multirackNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				count, err := utils.CountReadyPods(ctx, k8sClient, clusterName, multirackNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(count).To(Equal(2))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())

			By("scaling to 4 nodes")
			Expect(utils.PatchCluster(ctx, k8sClient, clusterName, multirackNS,
				[]byte(`{"spec":{"size":4}}`))).To(Succeed())

			By("waiting for 4 pods to be ready")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, multirackNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				count, err := utils.CountReadyPods(ctx, k8sClient, clusterName, multirackNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(count).To(Equal(4))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())

			By("verifying pods are distributed evenly after scale up")
			Eventually(func(g Gomega) {
				podList, err := utils.ListClusterPods(ctx, k8sClient, clusterName, multirackNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(podList.Items).To(HaveLen(4))

				rackCounts := map[string]int{}
				for _, pod := range podList.Items {
					rack := pod.Labels["acko.io/rack"]
					g.Expect(rack).NotTo(BeEmpty(), "pod %s should have rack label", pod.Name)
					rackCounts[rack]++
				}
				g.Expect(rackCounts).To(HaveLen(2))
				for rack, count := range rackCounts {
					g.Expect(count).To(Equal(2), "rack %s should have 2 pods after scale up", rack)
				}
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("verifying status.size reflects the new size")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, multirackNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Size).To(Equal(int32(4)))
			}, defaultTimeout, 2*time.Second).Should(Succeed())
		})
	})
})
