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

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/ksr/aerospike-ce-kubernetes-operator/test/utils"
)

const featuresNS = "aerospike-features"

var _ = Describe("Enhanced Features", Ordered, func() {

	BeforeAll(func() {
		By("creating features test namespace")
		Expect(utils.EnsureNamespace(ctx, k8sClient, featuresNS)).To(Succeed())
	})

	AfterAll(func() {
		By("cleaning up all feature test clusters")
		for _, name := range []string{
			"e2e-metrics", "e2e-podstatus", "e2e-config-change",
			"e2e-scale", "e2e-batch", "e2e-paused", "e2e-pdb",
		} {
			cmd := exec.Command("kubectl", "delete", "aerospikecluster", name,
				"-n", featuresNS, "--ignore-not-found", "--timeout=120s")
			_, _ = utils.Run(cmd)
		}
		By("deleting features test namespace")
		_ = utils.DeleteNamespace(ctx, k8sClient, featuresNS)
	})

	Context("Prometheus Custom Metrics", func() {
		const clusterName = "e2e-metrics"

		It("should expose custom business metrics after cluster creation", func() {
			By("creating a 1-node cluster")
			cluster := newTestCluster(clusterName, featuresNS, 1)
			// Retry creation in case the webhook isn't fully ready yet.
			Eventually(func() error {
				return k8sClient.Create(ctx, cluster)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("waiting for Completed phase")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("fetching fresh metrics after cluster reconciliation")
			refreshCurlMetricsPod()

			By("verifying custom metrics exist in the metrics endpoint")
			metricsOutput, err := getMetricsOutput()
			Expect(err).NotTo(HaveOccurred())
			Expect(metricsOutput).To(ContainSubstring("acko_cluster_phase"),
				"cluster phase metric should be present")
			Expect(metricsOutput).To(ContainSubstring("acko_cluster_ready_pods"),
				"ready pods metric should be present")
			Expect(metricsOutput).To(ContainSubstring("acko_reconcile_duration_seconds"),
				"reconcile duration metric should be present")
		})
	})

	Context("Per-Pod Status with ConfigHash and PodSpecHash", func() {
		const clusterName = "e2e-podstatus"

		It("should populate configHash and podSpecHash in status.pods", func() {
			By("creating a 1-node cluster")
			cluster := newTestCluster(clusterName, featuresNS, 1)
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())

			By("waiting for Completed phase")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("verifying status.pods has configHash and podSpecHash")
			Eventually(func(g Gomega) {
				cluster, err := utils.GetCluster(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(cluster.Status.Pods).To(HaveLen(1))

				for podName, ps := range cluster.Status.Pods {
					g.Expect(ps.ConfigHash).NotTo(BeEmpty(),
						"configHash should not be empty for pod %s", podName)
					g.Expect(ps.PodSpecHash).NotTo(BeEmpty(),
						"podSpecHash should not be empty for pod %s", podName)
					g.Expect(ps.IsRunningAndReady).To(BeTrue())
				}
			}, defaultTimeout, 2*time.Second).Should(Succeed())
		})

		It("should have matching config hash between pod annotation and status", func() {
			Eventually(func(g Gomega) {
				podList, err := utils.ListClusterPods(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(podList.Items).NotTo(BeEmpty())

				cluster, err := utils.GetCluster(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())

				for _, pod := range podList.Items {
					annotationHash := pod.Annotations["acko.io/config-hash"]
					g.Expect(annotationHash).NotTo(BeEmpty())

					if ps, ok := cluster.Status.Pods[pod.Name]; ok {
						g.Expect(ps.ConfigHash).To(Equal(annotationHash),
							"status configHash should match pod annotation for %s", pod.Name)
					}
				}
			}, defaultTimeout, 2*time.Second).Should(Succeed())
		})
	})

	Context("Config Change triggers Rolling Restart", Label("heavy"), func() {
		const clusterName = "e2e-config-change"

		It("should update pods when config changes", func() {
			By("creating a 2-node cluster")
			cluster := newTestCluster(clusterName, featuresNS, 2)
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())

			By("waiting for Completed phase")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				count, err := utils.CountReadyPods(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(count).To(Equal(2))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())

			By("recording current configHash values")
			oldHashes := map[string]string{}
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Pods).To(HaveLen(2))
				for name, ps := range c.Status.Pods {
					g.Expect(ps.ConfigHash).NotTo(BeEmpty(),
						"configHash should be populated for pod %s", name)
					oldHashes[name] = ps.ConfigHash
				}
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("patching proto-fd-max from 15000 to 20000")
			patch := `{"spec":{"aerospikeConfig":{"service":{"proto-fd-max":20000}}}}`
			Expect(utils.PatchCluster(ctx, k8sClient, clusterName, featuresNS, []byte(patch))).To(Succeed())

			By("waiting for cluster to return to Completed phase after config change")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
				// Verify configHash actually changed (not just Completed from before the patch)
				for name, ps := range c.Status.Pods {
					if oldHash, ok := oldHashes[name]; ok {
						g.Expect(ps.ConfigHash).NotTo(Equal(oldHash),
							"configHash should change for pod %s after config update", name)
					}
				}
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())
		})
	})

	Context("Scale Up and Down", Label("heavy"), func() {
		const clusterName = "e2e-scale"

		It("should scale up from 1 to 2 nodes", func() {
			By("creating a 1-node cluster")
			cluster := newTestCluster(clusterName, featuresNS, 1)
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())

			By("waiting for Completed phase with 1 pod")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				count, err := utils.CountReadyPods(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(count).To(Equal(1))
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("scaling up to 2 nodes")
			Expect(utils.PatchCluster(ctx, k8sClient, clusterName, featuresNS, []byte(`{"spec":{"size":2}}`))).To(Succeed())

			By("waiting for 2 pods to be ready")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				count, err := utils.CountReadyPods(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(count).To(Equal(2))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())

			By("verifying status.size is 2")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Size).To(Equal(int32(2)))
			}, defaultTimeout, 2*time.Second).Should(Succeed())
		})

		It("should scale down from 2 to 1 node", func() {
			By("scaling down to 1 node")
			Expect(utils.PatchCluster(ctx, k8sClient, clusterName, featuresNS, []byte(`{"spec":{"size":1}}`))).To(Succeed())

			By("waiting for Completed phase with 1 pod")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				count, err := utils.CountReadyPods(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(count).To(Equal(1))
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("verifying status.size is 1")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Size).To(Equal(int32(1)))
			}, defaultTimeout, 2*time.Second).Should(Succeed())
		})
	})

	Context("RollingUpdateBatchSize", Label("heavy"), func() {
		const clusterName = "e2e-batch"

		It("should handle batch rolling restart without errors", func() {
			By("creating a 2-node cluster with batchSize=2")
			cluster := newTestCluster(clusterName, featuresNS, 2, func(c *ackov1alpha1.AerospikeCluster) {
				batchSize := int32(2)
				c.Spec.RollingUpdateBatchSize = &batchSize
			})
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())

			By("waiting for Completed phase")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				count, err := utils.CountReadyPods(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(count).To(Equal(2))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())

			By("triggering a config change to force restart")
			patch := `{"spec":{"aerospikeConfig":{"service":{"proto-fd-max":18000}}}}`
			Expect(utils.PatchCluster(ctx, k8sClient, clusterName, featuresNS, []byte(patch))).To(Succeed())

			By("waiting for cluster to return to Completed (batch restart should work)")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())

			Eventually(func(g Gomega) {
				count, err := utils.CountReadyPods(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(count).To(Equal(2))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())
		})
	})

	Context("Paused Cluster", Label("heavy"), func() {
		const clusterName = "e2e-paused"

		It("should not reconcile config changes while paused", func() {
			By("creating a 1-node cluster")
			cluster := newTestCluster(clusterName, featuresNS, 1)
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())

			By("waiting for Completed phase")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("recording current configHash")
			var oldHash string
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Pods).NotTo(BeEmpty())
				for _, ps := range c.Status.Pods {
					g.Expect(ps.ConfigHash).NotTo(BeEmpty(), "configHash should be populated")
					oldHash = ps.ConfigHash
				}
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("pausing the cluster")
			Expect(utils.PatchCluster(ctx, k8sClient, clusterName, featuresNS, []byte(`{"spec":{"paused":true}}`))).To(Succeed())

			By("changing config while paused")
			patch := `{"spec":{"aerospikeConfig":{"service":{"proto-fd-max":25000}}}}`
			Expect(utils.PatchCluster(ctx, k8sClient, clusterName, featuresNS, []byte(patch))).To(Succeed())

			By("verifying configHash has NOT changed over 30 seconds (cluster is paused)")
			Consistently(func(g Gomega) {
				cluster, err := utils.GetCluster(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				for _, ps := range cluster.Status.Pods {
					g.Expect(ps.ConfigHash).To(Equal(oldHash),
						"configHash should not change while paused")
				}
			}, 30*time.Second, 5*time.Second).Should(Succeed())

			By("unpausing the cluster")
			Expect(utils.PatchCluster(ctx, k8sClient, clusterName, featuresNS, []byte(`{"spec":{"paused":false}}`))).To(Succeed())

			By("waiting for cluster to reconcile and return to Completed")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())

			By("verifying configHash has changed after unpause")
			Eventually(func(g Gomega) {
				cluster, err := utils.GetCluster(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				for _, ps := range cluster.Status.Pods {
					g.Expect(ps.ConfigHash).NotTo(Equal(oldHash),
						"configHash should change after unpause and config update")
				}
			}, defaultTimeout, 2*time.Second).Should(Succeed())
		})
	})

	Context("PodDisruptionBudget", Label("heavy"), func() {
		const clusterName = "e2e-pdb"

		It("should create PDB by default and delete when disabled", func() {
			By("creating a 2-node cluster")
			cluster := newTestCluster(clusterName, featuresNS, 2)
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())

			By("waiting for Completed phase")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())

			By("verifying PDB exists")
			Eventually(func(g Gomega) {
				exists, err := utils.PDBExists(ctx, k8sClient, fmt.Sprintf("%s-pdb", clusterName), featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(exists).To(BeTrue(), "PDB should exist for cluster")
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("disabling PDB")
			Expect(utils.PatchCluster(ctx, k8sClient, clusterName, featuresNS, []byte(`{"spec":{"disablePDB":true}}`))).To(Succeed())

			By("verifying PDB is deleted")
			Eventually(func(g Gomega) {
				exists, err := utils.PDBExists(ctx, k8sClient, fmt.Sprintf("%s-pdb", clusterName), featuresNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(exists).To(BeFalse(), "PDB should be deleted when disablePDB=true")
			}, defaultTimeout, 2*time.Second).Should(Succeed())
		})
	})
})
