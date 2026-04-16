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

	corev1 "k8s.io/api/core/v1"
	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/ksr/aerospike-ce-kubernetes-operator/test/utils"
)

// namespace where the project is deployed in
const namespace = "aerospike-operator"

// serviceAccountName created for the project
const serviceAccountName = "aerospike-ce-kubernetes-operator-controller-manager"

// metricsServiceName is the name of the metrics service of the project
const metricsServiceName = "aerospike-ce-kubernetes-operator-metrics-service"

// metricsRoleBindingName is the name of the RBAC that will be created to allow get the metrics data
const metricsRoleBindingName = "aerospike-ce-kubernetes-operator-metrics-binding"

var _ = Describe("Manager", Ordered, func() {
	var controllerPodName string

	// After each test, check for failures and collect logs, events,
	// and pod descriptions for debugging.
	AfterEach(func() {
		specReport := CurrentSpecReport()
		if specReport.Failed() {
			By("Fetching controller manager pod logs")
			cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
			controllerLogs, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Controller logs:\n %s", controllerLogs)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Controller logs: %s", err)
			}

			By("Fetching Kubernetes events")
			cmd = exec.Command("kubectl", "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
			eventsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Kubernetes events:\n%s", eventsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get Kubernetes events: %s", err)
			}

			By("Fetching curl-metrics logs")
			cmd = exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
			metricsOutput, err := utils.Run(cmd)
			if err == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "Metrics logs:\n %s", metricsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get curl-metrics logs: %s", err)
			}

			By("Fetching controller manager pod description")
			cmd = exec.Command("kubectl", "describe", "pod", controllerPodName, "-n", namespace)
			podDescription, err := utils.Run(cmd)
			if err == nil {
				fmt.Println("Pod description:\n", podDescription)
			} else {
				fmt.Println("Failed to describe controller pod")
			}
		}
	})

	SetDefaultEventuallyTimeout(2 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("Manager", func() {
		It("should run successfully", func() {
			By("validating that the controller-manager pod is running as expected")
			verifyControllerUp := func(g Gomega) {
				podList := &corev1.PodList{}
				err := k8sClient.List(ctx, podList,
					client.InNamespace(namespace),
					client.MatchingLabels{"control-plane": "controller-manager"},
				)
				g.Expect(err).NotTo(HaveOccurred(), "Failed to list controller-manager pods")

				// Filter out pods being deleted
				var activePods []corev1.Pod
				for _, p := range podList.Items {
					if p.DeletionTimestamp == nil {
						activePods = append(activePods, p)
					}
				}
				g.Expect(activePods).To(HaveLen(1), "expected 1 controller pod running")
				controllerPodName = activePods[0].Name
				g.Expect(controllerPodName).To(ContainSubstring("controller-manager"))
				g.Expect(activePods[0].Status.Phase).To(Equal(corev1.PodRunning),
					"Incorrect controller-manager pod status")
			}
			Eventually(verifyControllerUp).Should(Succeed())
		})

		It("should ensure the metrics endpoint is serving metrics", func() {
			By("validating that the metrics service is available")
			Eventually(func(g Gomega) {
				exists, err := utils.ServiceExists(ctx, k8sClient, metricsServiceName, namespace)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(exists).To(BeTrue(), "Metrics service should exist")
			}, 2*time.Minute, time.Second).Should(Succeed())

			By("getting the service account token")
			token, err := serviceAccountToken()
			Expect(err).NotTo(HaveOccurred())
			Expect(token).NotTo(BeEmpty())

			By("ensuring the controller pod is ready")
			Expect(controllerPodName).NotTo(BeEmpty(), "controllerPodName not set; did the first It block run?")
			Eventually(func(g Gomega) {
				pod := &corev1.Pod{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: controllerPodName, Namespace: namespace}, pod)
				g.Expect(err).NotTo(HaveOccurred())
				for _, cond := range pod.Status.Conditions {
					if cond.Type == corev1.PodReady {
						g.Expect(cond.Status).To(Equal(corev1.ConditionTrue), "Controller pod not ready")
						return
					}
				}
				g.Expect(false).To(BeTrue(), "Ready condition not found")
			}, 3*time.Minute, time.Second).Should(Succeed())

			By("verifying that the controller manager is serving the metrics server")
			verifyMetricsServerStarted := func(g Gomega) {
				cmd := exec.Command("kubectl", "logs", controllerPodName, "-n", namespace)
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(ContainSubstring("Serving metrics server"),
					"Metrics server not yet started")
			}
			Eventually(verifyMetricsServerStarted, 3*time.Minute, time.Second).Should(Succeed())

			// +kubebuilder:scaffold:e2e-metrics-webhooks-readiness

			By("cleaning up any existing curl-metrics pod")
			cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics",
				"-n", namespace, "--ignore-not-found", "--wait=true")
			_, _ = utils.Run(cmd)

			By("creating the curl-metrics pod to access the metrics endpoint")
			cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
				"--namespace", namespace,
				"--image=curlimages/curl:latest",
				"--overrides", curlMetricsPodOverrides(token))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

			By("waiting for the curl-metrics pod to complete.")
			verifyCurlUp := func(g Gomega) {
				pod := &corev1.Pod{}
				err := k8sClient.Get(ctx, client.ObjectKey{Name: "curl-metrics", Namespace: namespace}, pod)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(pod.Status.Phase).To(Equal(corev1.PodSucceeded), "curl pod in wrong status")
			}
			Eventually(verifyCurlUp, 5*time.Minute).Should(Succeed())

			By("getting the metrics by checking curl-metrics logs")
			verifyMetricsAvailable := func(g Gomega) {
				metricsOutput, err := getMetricsOutput()
				g.Expect(err).NotTo(HaveOccurred(), "Failed to retrieve logs from curl pod")
				g.Expect(metricsOutput).NotTo(BeEmpty())
				g.Expect(metricsOutput).To(ContainSubstring("< HTTP/1.1 200 OK"))
			}
			Eventually(verifyMetricsAvailable, 2*time.Minute).Should(Succeed())
		})

		// +kubebuilder:scaffold:e2e-webhooks-checks
	})
})

// serviceAccountToken returns a token for the specified service account
// using the Kubernetes TokenRequest API via client-go.
func serviceAccountToken() (string, error) {
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return "", fmt.Errorf("creating clientset: %w", err)
	}

	tokenReq, err := clientset.CoreV1().ServiceAccounts(namespace).CreateToken(
		ctx,
		serviceAccountName,
		&authv1.TokenRequest{},
		metav1.CreateOptions{},
	)
	if err != nil {
		return "", fmt.Errorf("creating token: %w", err)
	}
	return tokenReq.Status.Token, nil
}

// getMetricsOutput retrieves and returns the logs from the curl pod used to access the metrics endpoint.
func getMetricsOutput() (string, error) {
	By("getting the curl-metrics logs")
	cmd := exec.Command("kubectl", "logs", "curl-metrics", "-n", namespace)
	return utils.Run(cmd)
}

// curlMetricsPodOverrides returns the JSON overrides for a curl-metrics pod.
func curlMetricsPodOverrides(token string) string {
	return fmt.Sprintf(`{
		"spec": {
			"containers": [{
				"name": "curl",
				"image": "curlimages/curl:latest",
				"command": ["/bin/sh", "-c"],
				"args": [
					"for i in $(seq 1 30); do curl -v -k -H 'Authorization: Bearer %s' https://%s.%s.svc.cluster.local:8443/metrics && exit 0 || sleep 2; done; exit 1"
				],
				"securityContext": {
					"readOnlyRootFilesystem": true,
					"allowPrivilegeEscalation": false,
					"capabilities": {
						"drop": ["ALL"]
					},
					"runAsNonRoot": true,
					"runAsUser": 1000,
					"seccompProfile": {
						"type": "RuntimeDefault"
					}
				}
			}],
			"serviceAccountName": "%s"
		}
	}`, token, metricsServiceName, namespace, serviceAccountName)
}

// refreshCurlMetricsPod deletes the existing curl-metrics pod and creates a new
// one to fetch fresh metrics from the metrics endpoint.
func refreshCurlMetricsPod() {
	By("deleting old curl-metrics pod")
	cmd := exec.Command("kubectl", "delete", "pod", "curl-metrics", "-n", namespace, "--ignore-not-found", "--wait=true")
	_, _ = utils.Run(cmd)

	token, err := serviceAccountToken()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	By("creating fresh curl-metrics pod")
	cmd = exec.Command("kubectl", "run", "curl-metrics", "--restart=Never",
		"--namespace", namespace,
		"--image=curlimages/curl:latest",
		"--overrides", curlMetricsPodOverrides(token))
	_, err = utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to create curl-metrics pod")

	By("waiting for curl-metrics pod to complete")
	Eventually(func(g Gomega) {
		pod := &corev1.Pod{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: "curl-metrics", Namespace: namespace}, pod)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(pod.Status.Phase).To(Equal(corev1.PodSucceeded), "curl pod in wrong status")
	}, 2*time.Minute, time.Second).Should(Succeed())
}
