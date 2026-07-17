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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/test/utils"
)

const templateTestNS = "aerospike-template"

var _ = Describe("Cluster templates", Ordered, Label("heavy"), func() {

	BeforeAll(func() {
		By("creating template test namespace")
		Expect(utils.EnsureNamespace(ctx, k8sClient, templateTestNS)).To(Succeed())
	})

	AfterAll(func() {
		By("cleaning up all template test clusters")
		for _, name := range []string{"e2e-tpl-basic", "e2e-tpl-drift"} {
			cmd := exec.Command("kubectl", "delete", "aerospikecluster", name,
				"-n", templateTestNS, "--ignore-not-found", "--timeout=120s")
			_, _ = utils.Run(cmd)
		}
		By("cleaning up templates")
		for _, name := range []string{"e2e-template", "e2e-template-drift"} {
			cmd := exec.Command("kubectl", "delete", "aerospikeclustertemplate", name,
				"--ignore-not-found", "--timeout=60s")
			_, _ = utils.Run(cmd)
		}
		By("deleting template test namespace")
		_ = utils.DeleteNamespace(ctx, k8sClient, templateTestNS)
	})

	Context("Create cluster from template", func() {
		const (
			templateName = "e2e-template"
			clusterName  = "e2e-tpl-basic"
		)

		It("should create a cluster from a template", func() {
			By("creating the template")
			protoFdMax := float64(8000)
			template := &ackov1alpha1.AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: templateName,
				},
				Spec: ackov1alpha1.AerospikeClusterTemplateSpec{
					Image: "aerospike:ce-8.1.1.1",
					AerospikeConfig: &ackov1alpha1.TemplateAerospikeConfig{
						Service: &ackov1alpha1.AerospikeConfigSpec{
							Value: map[string]any{
								"proto-fd-max": protoFdMax,
							},
						},
						NamespaceDefaults: &ackov1alpha1.AerospikeConfigSpec{
							Value: map[string]any{
								"replication-factor": float64(1),
							},
						},
					},
				},
			}
			Eventually(func() error {
				return k8sClient.Create(ctx, template)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("creating a cluster that references the template")
			cluster := newTestCluster(clusterName, templateTestNS, 1, func(c *ackov1alpha1.AerospikeCluster) {
				c.Spec.TemplateRef = &ackov1alpha1.TemplateRef{
					Name: templateName,
				}
			})
			Eventually(func() error {
				return k8sClient.Create(ctx, cluster)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("waiting for Completed phase")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, templateTestNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("verifying the cluster has a template snapshot in status")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, templateTestNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.TemplateSnapshot).NotTo(BeNil(),
					"templateSnapshot should be populated")
				g.Expect(c.Status.TemplateSnapshot.Name).To(Equal(templateName))
				g.Expect(c.Status.TemplateSnapshot.Synced).To(BeTrue(),
					"template snapshot should be synced initially")
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("verifying 1 pod is running and ready")
			Eventually(func(g Gomega) {
				count, err := utils.CountReadyPods(ctx, k8sClient, clusterName, templateTestNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(count).To(Equal(1))
			}, defaultTimeout, 2*time.Second).Should(Succeed())
		})
	})

	Context("Template drift detection", func() {
		const (
			templateName = "e2e-template-drift"
			clusterName  = "e2e-tpl-drift"
		)

		It("should detect template drift after template modification", func() {
			By("creating the template")
			template := &ackov1alpha1.AerospikeClusterTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: templateName,
				},
				Spec: ackov1alpha1.AerospikeClusterTemplateSpec{
					Image: "aerospike:ce-8.1.1.1",
					AerospikeConfig: &ackov1alpha1.TemplateAerospikeConfig{
						Service: &ackov1alpha1.AerospikeConfigSpec{
							Value: map[string]any{
								"proto-fd-max": float64(10000),
							},
						},
						NamespaceDefaults: &ackov1alpha1.AerospikeConfigSpec{
							Value: map[string]any{
								"replication-factor": float64(1),
							},
						},
					},
				},
			}
			Eventually(func() error {
				return k8sClient.Create(ctx, template)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("creating a cluster with templateRef")
			cluster := newTestCluster(clusterName, templateTestNS, 1, func(c *ackov1alpha1.AerospikeCluster) {
				c.Spec.TemplateRef = &ackov1alpha1.TemplateRef{
					Name: templateName,
				}
			})
			Eventually(func() error {
				return k8sClient.Create(ctx, cluster)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("waiting for Completed phase")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, templateTestNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.Phase).To(Equal(ackov1alpha1.AerospikePhaseCompleted))
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("verifying snapshot is initially synced and recording resourceVersion")
			var oldResourceVersion string
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, templateTestNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.TemplateSnapshot).NotTo(BeNil())
				g.Expect(c.Status.TemplateSnapshot.Synced).To(BeTrue())
				oldResourceVersion = c.Status.TemplateSnapshot.ResourceVersion
			}, defaultTimeout, 2*time.Second).Should(Succeed())
			Expect(oldResourceVersion).NotTo(BeEmpty(), "oldResourceVersion should have been captured")

			By("modifying the template to trigger drift")
			Expect(utils.PatchTemplate(ctx, k8sClient, templateName,
				[]byte(`{"spec":{"aerospikeConfig":{"service":{"proto-fd-max":12000}}}}`))).To(Succeed())

			By("waiting for cluster status to show synced=false (drift detected)")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, templateTestNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.TemplateSnapshot).NotTo(BeNil())
				g.Expect(c.Status.TemplateSnapshot.Synced).To(BeFalse(),
					"template snapshot should be out of sync after template change")
			}, defaultTimeout, 2*time.Second).Should(Succeed())

			By("verifying the snapshot still references the old resourceVersion")
			Eventually(func(g Gomega) {
				c, err := utils.GetCluster(ctx, k8sClient, clusterName, templateTestNS)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(c.Status.TemplateSnapshot).NotTo(BeNil())
				g.Expect(c.Status.TemplateSnapshot.ResourceVersion).To(Equal(oldResourceVersion),
					"snapshot resourceVersion should not change until resync")
			}, defaultTimeout, 2*time.Second).Should(Succeed())
		})
	})
})
