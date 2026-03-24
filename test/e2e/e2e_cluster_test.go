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
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/ksr/aerospike-ce-kubernetes-operator/test/utils"
)

const (
	aerospikeNS      = "aerospike"
	defaultTimeout   = 3 * time.Minute
	multiNodeTimeout = 5 * time.Minute
)

var _ = Describe("AerospikeCluster Samples", Ordered, func() {
	var projectDir string

	BeforeAll(func() {
		var err error
		projectDir, err = utils.GetProjectDir()
		Expect(err).NotTo(HaveOccurred())

		By("creating aerospike namespace")
		Expect(utils.EnsureNamespace(ctx, k8sClient, aerospikeNS)).To(Succeed())
	})

	AfterAll(func() {
		By("cleaning up all sample clusters")
		for _, name := range []string{"aerospike-basic", "aerospike-3node", "aerospike-multirack", "aerospike-acl"} {
			// Use kubectl delete with timeout to wait for finalizer cleanup,
			// ensuring the operator finishes reconciling before the next suite starts.
			cmd := exec.Command("kubectl", "delete", "aerospikecluster", name,
				"-n", aerospikeNS, "--ignore-not-found", "--timeout=120s")
			_, _ = utils.Run(cmd)
		}
		By("deleting aerospike namespace")
		_ = utils.DeleteNamespace(ctx, k8sClient, aerospikeNS)
	})

	Context("Basic single-node cluster", func() {
		const clusterName = "aerospike-basic"

		It("should deploy and reach Completed phase", func() {
			samplePath := filepath.Join(projectDir, "config", "samples", "acko_v1alpha1_aerospikecluster.yaml")

			By("loading and creating the basic sample CR")
			cluster, err := loadClusterFromFile(samplePath)
			Expect(err).NotTo(HaveOccurred())
			// Retry creation in case the webhook isn't fully ready yet.
			Eventually(func() error {
				return k8sClient.Create(ctx, cluster)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("waiting for Completed phase")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, aerospikeNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("verifying 1 pod is running and ready")
			Eventually(func(g Gomega) {
				count, err := utils.CountReadyPods(ctx, k8sClient, clusterName, aerospikeNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(count).To(Equal(1))
			}, defaultTimeout, 2*time.Second).Should(Succeed())
		})

		It("should create expected Kubernetes resources", func() {
			By("verifying headless service exists")
			exists, err := utils.ServiceExists(ctx, k8sClient, clusterName, aerospikeNS)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue(), "headless service should exist")

			By("verifying StatefulSet exists with correct name")
			stsList, err := utils.ListClusterStatefulSets(ctx, k8sClient, clusterName, aerospikeNS)
			Expect(err).NotTo(HaveOccurred())
			Expect(stsList.Items).To(HaveLen(1))
			Expect(stsList.Items[0].Name).To(Equal(fmt.Sprintf("%s-0", clusterName)))

			By("verifying ConfigMap exists")
			exists, err = utils.ConfigMapExists(ctx, k8sClient, fmt.Sprintf("%s-0-config", clusterName), aerospikeNS)
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue(), "configmap should exist")
		})

		It("should populate pod status correctly", func() {
			By("verifying status.pods has 1 entry with IsRunningAndReady=true")
			Eventually(func(g Gomega) {
				cluster, err := utils.GetCluster(ctx, k8sClient, clusterName, aerospikeNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(cluster.Status.Pods).To(HaveLen(1))
				for _, ps := range cluster.Status.Pods {
					g.Expect(ps.IsRunningAndReady).To(BeTrue())
					g.Expect(ps.Image).To(Equal("aerospike:ce-8.1.1.1"))
				}
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("verifying status.size is 1")
			cluster, err := utils.GetCluster(ctx, k8sClient, clusterName, aerospikeNS)
			Expect(err).NotTo(HaveOccurred())
			Expect(cluster.Status.Size).To(Equal(int32(1)))
		})
	})

	Context("3-node cluster with PV storage", Label("heavy"), func() {
		const clusterName = "aerospike-3node"

		It("should deploy 3 nodes and reach Completed phase", func() {
			samplePath := filepath.Join(projectDir, "config", "samples", "aerospike-cluster-3node.yaml")

			By("loading and creating the 3-node sample CR")
			cluster, err := loadClusterFromFile(samplePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())

			By("waiting for Completed phase")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, aerospikeNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())

			By("verifying 3 pods are running and ready")
			Eventually(func(g Gomega) {
				count, err := utils.CountReadyPods(ctx, k8sClient, clusterName, aerospikeNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(count).To(Equal(3))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())
		})

		It("should create PVCs for each pod", func() {
			By("verifying PVCs are created")
			Eventually(func(g Gomega) {
				pvcList, err := utils.ListClusterPVCs(ctx, k8sClient, clusterName, aerospikeNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(pvcList.Items)).To(BeNumerically(">=", 3),
					"should have at least 3 PVCs for 3 pods")
			}, defaultTimeout, 2*time.Second).Should(Succeed())
		})

		It("should report correct status", func() {
			By("verifying status.size is 3")
			cluster, err := utils.GetCluster(ctx, k8sClient, clusterName, aerospikeNS)
			Expect(err).NotTo(HaveOccurred())
			Expect(cluster.Status.Size).To(Equal(int32(3)))

			By("verifying all pods have correct image in status")
			Expect(cluster.Status.Pods).To(HaveLen(3))
			for _, ps := range cluster.Status.Pods {
				Expect(ps.Image).To(Equal("aerospike:ce-8.1.1.1"))
				Expect(ps.IsRunningAndReady).To(BeTrue())
			}
		})
	})

	Context("Multi-rack 6-node cluster", Label("heavy"), func() {
		const clusterName = "aerospike-multirack"

		It("should deploy 6 nodes across 3 racks and reach Completed phase", func() {
			samplePath := filepath.Join(projectDir, "config", "samples", "aerospike-cluster-multirack.yaml")

			By("loading and creating the multi-rack sample CR")
			cluster, err := loadClusterFromFile(samplePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())

			By("waiting for Completed phase")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, aerospikeNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())

			By("verifying 6 pods are running and ready")
			Eventually(func(g Gomega) {
				count, err := utils.CountReadyPods(ctx, k8sClient, clusterName, aerospikeNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(count).To(Equal(6))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())
		})

		It("should create 3 StatefulSets (one per rack)", func() {
			stsList, err := utils.ListClusterStatefulSets(ctx, k8sClient, clusterName, aerospikeNS)
			Expect(err).NotTo(HaveOccurred())
			Expect(stsList.Items).To(HaveLen(3))

			stsNames := make([]string, 0, len(stsList.Items))
			for _, sts := range stsList.Items {
				stsNames = append(stsNames, sts.Name)
			}
			Expect(stsNames).To(ContainElements(
				fmt.Sprintf("%s-1", clusterName),
				fmt.Sprintf("%s-2", clusterName),
				fmt.Sprintf("%s-3", clusterName),
			))
		})

		It("should assign rack labels to pods", func() {
			podList, err := utils.ListClusterPods(ctx, k8sClient, clusterName, aerospikeNS)
			Expect(err).NotTo(HaveOccurred())
			Expect(podList.Items).To(HaveLen(6))

			rackCounts := map[string]int{}
			for _, pod := range podList.Items {
				rack := pod.Labels["acko.io/rack"]
				Expect(rack).NotTo(BeEmpty(), "pod %s should have rack label", pod.Name)
				rackCounts[rack]++
			}

			By("verifying each rack has 2 pods")
			Expect(rackCounts).To(HaveLen(3))
			for rack, count := range rackCounts {
				Expect(count).To(Equal(2), "rack %s should have 2 pods", rack)
			}
		})

		It("should create 3 ConfigMaps (one per rack)", func() {
			for _, rackID := range []int{1, 2, 3} {
				cmName := fmt.Sprintf("%s-%d-config", clusterName, rackID)
				exists, err := utils.ConfigMapExists(ctx, k8sClient, cmName, aerospikeNS)
				Expect(err).NotTo(HaveOccurred())
				Expect(exists).To(BeTrue(), "configmap %s should exist", cmName)
			}
		})

		It("should report correct status.size", func() {
			cluster, err := utils.GetCluster(ctx, k8sClient, clusterName, aerospikeNS)
			Expect(err).NotTo(HaveOccurred())
			Expect(cluster.Status.Size).To(Equal(int32(6)))
		})
	})

	Context("ACL/Storage sample with cascadeDelete", Label("heavy"), func() {
		const clusterName = "aerospike-acl"

		It("should deploy 3 nodes and reach Completed phase", func() {
			samplePath := filepath.Join(projectDir, "config", "samples", "aerospike-cluster-acl.yaml")

			By("creating required Kubernetes Secrets for ACL users")
			for _, secret := range []struct{ name, password string }{
				{"aerospike-admin-secret", "admin123"},
				{"aerospike-reader-secret", "reader123"},
			} {
				cmd := exec.Command("kubectl", "-n", aerospikeNS, "create", "secret", "generic",
					secret.name, fmt.Sprintf("--from-literal=password=%s", secret.password))
				_, err := utils.Run(cmd)
				if err != nil && !strings.Contains(err.Error(), "already exists") {
					Expect(err).NotTo(HaveOccurred(), "failed to create secret %s", secret.name)
				}
			}

			By("loading and creating the ACL sample CR")
			cluster, err := loadClusterFromFile(samplePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Create(ctx, cluster)).To(Succeed())

			By("waiting for Completed phase")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, aerospikeNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())

			By("verifying 3 pods are running and ready")
			Eventually(func(g Gomega) {
				count, err := utils.CountReadyPods(ctx, k8sClient, clusterName, aerospikeNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(count).To(Equal(3))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())
		})

		It("should have PVCs", func() {
			Eventually(func(g Gomega) {
				pvcList, err := utils.ListClusterPVCs(ctx, k8sClient, clusterName, aerospikeNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(pvcList.Items)).To(BeNumerically(">=", 3))
			}, defaultTimeout, 2*time.Second).Should(Succeed())
		})

		It("should delete PVCs when cluster is deleted (cascadeDelete)", func() {
			By("deleting the cluster")
			Expect(utils.DeleteCluster(ctx, k8sClient, clusterName, aerospikeNS)).To(Succeed())

			By("waiting for PVCs to be cleaned up")
			Eventually(func(g Gomega) {
				pvcList, err := utils.ListClusterPVCs(ctx, k8sClient, clusterName, aerospikeNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(pvcList.Items).To(BeEmpty(), "PVCs should be deleted with cascadeDelete=true")
			}, defaultTimeout, 2*time.Second).Should(Succeed())
		})
	})
})
