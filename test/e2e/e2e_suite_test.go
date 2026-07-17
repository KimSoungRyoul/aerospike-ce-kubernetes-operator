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
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/test/utils"
)

var (
	// managerImage is the manager image to be built and loaded for testing.
	managerImage = "ghcr.io/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator:v0.0.1"
	// shouldCleanupCertManager tracks whether CertManager was installed by this suite.
	shouldCleanupCertManager = false

	// k8sClient is a typed controller-runtime client for the Kind cluster.
	k8sClient client.Client
	// ctx is the shared context for all e2e test operations.
	ctx context.Context
	// restConfig is the REST config for the Kind cluster (used by clientset).
	restConfig *rest.Config
)

// TestE2E runs the e2e test suite to validate the solution in an isolated environment.
// The default setup requires Kind and CertManager.
//
// To skip CertManager installation, set: CERT_MANAGER_INSTALL_SKIP=true
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting aerospike-ce-kubernetes-operator e2e test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	By("building the manager image")
	buildArgs := []string{"docker-build", fmt.Sprintf("IMG=%s", managerImage)}
	if ct := os.Getenv("CONTAINER_TOOL"); ct != "" {
		buildArgs = append(buildArgs, fmt.Sprintf("CONTAINER_TOOL=%s", ct))
	}
	cmd := exec.Command("make", buildArgs...)
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager image")

	By("loading the manager image on Kind")
	err = utils.LoadImageToKindClusterWithName(managerImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager image into Kind")

	setupCertManager()

	By("creating manager namespace (idempotent)")
	cmd = exec.Command("kubectl", "create", "ns", namespace, "--dry-run=client", "-o", "yaml")
	nsYAML, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to render manager namespace YAML")
	cmd = exec.Command("kubectl", "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(string(nsYAML))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create manager namespace")

	By("labeling the namespace to enforce the restricted security policy")
	cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
		"pod-security.kubernetes.io/enforce=restricted")
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to label namespace")

	By("installing CRDs")
	cmd = exec.Command("make", "install")
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to install CRDs")

	By("deploying the controller-manager")
	cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", managerImage))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")

	By("setting up controller-runtime client")
	ctx = context.Background()
	err = ackov1alpha1.AddToScheme(scheme.Scheme)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to register CRD scheme")

	restConfig, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load kubeconfig")

	k8sClient, err = client.New(restConfig, client.Options{Scheme: scheme.Scheme})
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create k8s client")

	By("waiting for the controller-manager pod to be ready")
	Eventually(func(g Gomega) {
		podList := &corev1.PodList{}
		err := k8sClient.List(ctx, podList,
			client.InNamespace(namespace),
			client.MatchingLabels{"control-plane": "controller-manager"},
		)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(podList.Items).To(HaveLen(1), "expected 1 controller pod")
		for _, cond := range podList.Items[0].Status.Conditions {
			if cond.Type == corev1.PodReady {
				g.Expect(cond.Status).To(Equal(corev1.ConditionTrue), "controller-manager pod not ready")
				return
			}
		}
		g.Expect(false).To(BeTrue(), "Ready condition not found on controller pod")
	}, 2*time.Minute, time.Second).Should(Succeed())

	By("waiting for webhook to be ready")
	Eventually(func(g Gomega) {
		vwc := &admissionv1.ValidatingWebhookConfiguration{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name: "aerospike-ce-kubernetes-operator-validating-webhook-configuration",
		}, vwc)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(vwc.Webhooks).NotTo(BeEmpty())
		g.Expect(vwc.Webhooks[0].ClientConfig.CABundle).NotTo(BeEmpty(), "webhook CA bundle not yet injected")
	}, 2*time.Minute, time.Second).Should(Succeed())

	By("creating ClusterRoleBinding for metrics access")
	cmd = exec.Command("kubectl", "create", "clusterrolebinding", metricsRoleBindingName,
		"--clusterrole=aerospike-ce-kubernetes-operator-metrics-reader",
		fmt.Sprintf("--serviceaccount=%s:%s", namespace, serviceAccountName),
	)
	_, _ = utils.Run(cmd) // ignore error if already exists
})

var _ = AfterSuite(func() {
	By("cleaning up the curl pod for metrics")
	cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace, "--ignore-not-found")
	_, _ = utils.Run(cmd)

	By("removing metrics ClusterRoleBinding")
	cmd = exec.Command("kubectl", "delete", "clusterrolebinding", metricsRoleBindingName, "--ignore-not-found")
	_, _ = utils.Run(cmd)

	By("undeploying the controller-manager")
	cmd = exec.Command("make", "undeploy")
	_, _ = utils.Run(cmd)

	By("uninstalling CRDs")
	cmd = exec.Command("make", "uninstall")
	_, _ = utils.Run(cmd)

	By("removing manager namespace")
	cmd = exec.Command("kubectl", "delete", "ns", namespace, "--ignore-not-found")
	_, _ = utils.Run(cmd)

	teardownCertManager()
})

// setupCertManager installs CertManager if needed for webhook tests.
// Skips installation if CERT_MANAGER_INSTALL_SKIP=true or if already present.
func setupCertManager() {
	if os.Getenv("CERT_MANAGER_INSTALL_SKIP") == "true" {
		_, _ = fmt.Fprintf(GinkgoWriter, "Skipping CertManager installation (CERT_MANAGER_INSTALL_SKIP=true)\n")
		return
	}

	By("checking if CertManager is already installed")
	if utils.IsCertManagerCRDsInstalled() {
		_, _ = fmt.Fprintf(GinkgoWriter, "CertManager is already installed. Skipping installation.\n")
		return
	}

	// Mark for cleanup before installation to handle interruptions and partial installs.
	shouldCleanupCertManager = true

	By("installing CertManager")
	Expect(utils.InstallCertManager()).To(Succeed(), "Failed to install CertManager")
}

// teardownCertManager uninstalls CertManager if it was installed by setupCertManager.
// This ensures we only remove what we installed.
func teardownCertManager() {
	if !shouldCleanupCertManager {
		_, _ = fmt.Fprintf(GinkgoWriter, "Skipping CertManager cleanup (not installed by this suite)\n")
		return
	}

	By("uninstalling CertManager")
	utils.UninstallCertManager()
}
