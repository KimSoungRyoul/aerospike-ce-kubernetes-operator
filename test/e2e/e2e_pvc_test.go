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
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/ksr/aerospike-ce-kubernetes-operator/test/utils"
)

const pvcNS = "aerospike-pvc"

// boolPtr returns a pointer to a bool value.
func boolPtr(b bool) *bool { return &b }

var _ = Describe("PVC management", Ordered, Label("heavy"), func() {

	BeforeAll(func() {
		By("creating PVC test namespace")
		Expect(utils.EnsureNamespace(ctx, k8sClient, pvcNS)).To(Succeed())
	})

	AfterAll(func() {
		By("cleaning up all PVC test clusters")
		for _, name := range []string{"e2e-pvc-create", "e2e-pvc-cascade", "e2e-pvc-retain"} {
			cmd := exec.Command("kubectl", "delete", "aerospikecluster", name,
				"-n", pvcNS, "--ignore-not-found", "--timeout=120s")
			_, _ = utils.Run(cmd)
		}
		By("deleting PVC test namespace")
		_ = utils.DeleteNamespace(ctx, k8sClient, pvcNS)
	})

	Context("PVC creation for storage volumes", func() {
		const clusterName = "e2e-pvc-create"

		It("should create PVCs for storage volumes", func() {
			By("creating a 2-node cluster with persistent storage")
			cluster := newTestCluster(clusterName, pvcNS, 2, func(c *ackov1alpha1.AerospikeCluster) {
				c.Spec.Storage = &ackov1alpha1.AerospikeStorageSpec{
					Volumes: []ackov1alpha1.VolumeSpec{
						{
							Name: "data-vol",
							Source: ackov1alpha1.VolumeSource{
								PersistentVolume: &ackov1alpha1.PersistentVolumeSpec{
									StorageClass: "standard",
									Size:         "1Gi",
								},
							},
							Aerospike: &ackov1alpha1.AerospikeVolumeAttachment{
								Path: "/opt/aerospike/data",
							},
							CascadeDelete: boolPtr(false),
						},
					},
				}
				// Configure the namespace to use in-memory so the cluster
				// can start without requiring device files on the PVC.
				c.Spec.AerospikeConfig = &ackov1alpha1.AerospikeConfigSpec{
					Value: map[string]any{
						"service": map[string]any{
							"cluster-name": clusterName,
							"proto-fd-max": float64(15000),
						},
						"network": map[string]any{
							"service":   map[string]any{"address": "any", "port": float64(3000)},
							"heartbeat": map[string]any{"mode": "mesh", "port": float64(3002)},
							"fabric":    map[string]any{"address": "any", "port": float64(3001)},
						},
						"namespaces": []any{
							map[string]any{
								"name":               "test",
								"replication-factor": float64(1),
								"storage-engine": map[string]any{
									"type":      "memory",
									"data-size": float64(1073741824),
								},
							},
						},
					},
				}
			})
			Eventually(func() error {
				return k8sClient.Create(ctx, cluster)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("waiting for Completed phase")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, pvcNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())

			By("verifying PVCs are created for each pod")
			Eventually(func(g Gomega) {
				pvcList, err := utils.ListClusterPVCs(ctx, k8sClient, clusterName, pvcNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(pvcList.Items)).To(BeNumerically(">=", 2),
					"should have at least 2 PVCs for 2 pods")
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("verifying 2 pods are running and ready")
			Eventually(func(g Gomega) {
				count, err := utils.CountReadyPods(ctx, k8sClient, clusterName, pvcNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(count).To(Equal(2))
			}, multiNodeTimeout, 2*time.Second).Should(Succeed())
		})

		It("should report correct status with storage", func() {
			Eventually(func(g Gomega) {
				cluster, err := utils.GetCluster(ctx, k8sClient, clusterName, pvcNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(cluster.Status.Size).To(Equal(int32(2)))
				g.Expect(cluster.Status.Pods).To(HaveLen(2))

				for _, ps := range cluster.Status.Pods {
					g.Expect(ps.IsRunningAndReady).To(BeTrue())
				}
			}, defaultTimeout, 2*time.Second).Should(Succeed())
		})
	})

	Context("CascadeDelete PVC cleanup", func() {
		const clusterName = "e2e-pvc-cascade"

		It("should clean up cascade-delete PVCs on cluster deletion", func() {
			By("creating a 1-node cluster with cascadeDelete=true")
			cluster := newTestCluster(clusterName, pvcNS, 1, func(c *ackov1alpha1.AerospikeCluster) {
				c.Spec.Storage = &ackov1alpha1.AerospikeStorageSpec{
					Volumes: []ackov1alpha1.VolumeSpec{
						{
							Name: "data-vol",
							Source: ackov1alpha1.VolumeSource{
								PersistentVolume: &ackov1alpha1.PersistentVolumeSpec{
									StorageClass: "standard",
									Size:         "1Gi",
								},
							},
							Aerospike: &ackov1alpha1.AerospikeVolumeAttachment{
								Path: "/opt/aerospike/data",
							},
							CascadeDelete: boolPtr(true),
						},
					},
				}
			})
			Eventually(func() error {
				return k8sClient.Create(ctx, cluster)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("waiting for Completed phase")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, pvcNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("verifying PVCs exist before deletion")
			Eventually(func(g Gomega) {
				pvcList, err := utils.ListClusterPVCs(ctx, k8sClient, clusterName, pvcNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(pvcList.Items)).To(BeNumerically(">=", 1),
					"should have at least 1 PVC")
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("deleting the cluster")
			Expect(utils.DeleteCluster(ctx, k8sClient, clusterName, pvcNS)).To(Succeed())

			By("waiting for PVCs to be cleaned up (cascadeDelete=true)")
			Eventually(func(g Gomega) {
				pvcList, err := utils.ListClusterPVCs(ctx, k8sClient, clusterName, pvcNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(pvcList.Items).To(BeEmpty(),
					"PVCs should be deleted with cascadeDelete=true")
			}, defaultTimeout, 2*time.Second).Should(Succeed())
		})
	})

	Context("Retained PVCs without cascadeDelete", func() {
		const clusterName = "e2e-pvc-retain"

		It("should retain PVCs when cascadeDelete is false", func() {
			By("creating a 1-node cluster with cascadeDelete=false")
			cluster := newTestCluster(clusterName, pvcNS, 1, func(c *ackov1alpha1.AerospikeCluster) {
				c.Spec.Storage = &ackov1alpha1.AerospikeStorageSpec{
					Volumes: []ackov1alpha1.VolumeSpec{
						{
							Name: "data-vol",
							Source: ackov1alpha1.VolumeSource{
								PersistentVolume: &ackov1alpha1.PersistentVolumeSpec{
									StorageClass: "standard",
									Size:         "1Gi",
								},
							},
							Aerospike: &ackov1alpha1.AerospikeVolumeAttachment{
								Path: "/opt/aerospike/data",
							},
							CascadeDelete: boolPtr(false),
						},
					},
				}
			})
			Eventually(func() error {
				return k8sClient.Create(ctx, cluster)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("waiting for Completed phase")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, pvcNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("verifying PVCs exist")
			Eventually(func(g Gomega) {
				pvcList, err := utils.ListClusterPVCs(ctx, k8sClient, clusterName, pvcNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(pvcList.Items)).To(BeNumerically(">=", 1))
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("deleting the cluster")
			Expect(utils.DeleteCluster(ctx, k8sClient, clusterName, pvcNS)).To(Succeed())

			By("waiting for the cluster CR to be gone")
			Eventually(func(g Gomega) {
				_, err := utils.GetCluster(ctx, k8sClient, clusterName, pvcNS)
				g.Expect(err).To(HaveOccurred(), "cluster should be deleted")
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("verifying PVCs are retained (cascadeDelete=false)")
			// Wait a bit and then verify PVCs still exist.
			Consistently(func(g Gomega) {
				pvcList, err := utils.ListClusterPVCs(ctx, k8sClient, clusterName, pvcNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(len(pvcList.Items)).To(BeNumerically(">=", 1),
					"PVCs should be retained when cascadeDelete=false")
			}, 15*time.Second, 3*time.Second).Should(Succeed())

			By("cleaning up retained PVCs manually")
			cmd := exec.Command("kubectl", "delete", "pvc", "-l",
				"acko.io/cluster="+clusterName, "-n", pvcNS, "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})
	})
})
