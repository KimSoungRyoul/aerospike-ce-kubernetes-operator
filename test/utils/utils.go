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

package utils

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2" // nolint:revive,staticcheck
)

const (
	certmanagerVersion = "v1.19.3"
	certmanagerURLTmpl = "https://github.com/cert-manager/cert-manager/releases/download/%s/cert-manager.yaml"

	defaultKindBinary  = "kind"
	defaultKindCluster = "kind"
	podmanRuntime      = "podman"
)

func warnError(err error) {
	_, _ = fmt.Fprintf(GinkgoWriter, "warning: %v\n", err)
}

// Run executes the provided command within this context
func Run(cmd *exec.Cmd) (string, error) {
	dir, _ := GetProjectDir()
	cmd.Dir = dir

	if err := os.Chdir(cmd.Dir); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "chdir dir: %q\n", err)
	}

	cmd.Env = append(os.Environ(), "GO111MODULE=on")
	command := strings.Join(cmd.Args, " ")
	_, _ = fmt.Fprintf(GinkgoWriter, "running: %q\n", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%q failed with error %q: %w", command, string(output), err)
	}

	return string(output), nil
}

// UninstallCertManager uninstalls the cert manager
func UninstallCertManager() {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command("kubectl", "delete", "-f", url)
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}

	// Delete leftover leases in kube-system (not cleaned by default)
	kubeSystemLeases := []string{
		"cert-manager-cainjector-leader-election",
		"cert-manager-controller",
	}
	for _, lease := range kubeSystemLeases {
		cmd = exec.Command("kubectl", "delete", "lease", lease,
			"-n", "kube-system", "--ignore-not-found", "--force", "--grace-period=0")
		if _, err := Run(cmd); err != nil {
			warnError(err)
		}
	}
}

// InstallCertManager installs the cert manager bundle.
func InstallCertManager() error {
	url := fmt.Sprintf(certmanagerURLTmpl, certmanagerVersion)
	cmd := exec.Command("kubectl", "apply", "-f", url)
	if _, err := Run(cmd); err != nil {
		return err
	}
	// Wait for cert-manager-webhook to be ready, which can take time if cert-manager
	// was re-installed after uninstalling on a cluster.
	cmd = exec.Command("kubectl", "wait", "deployment.apps/cert-manager-webhook",
		"--for", "condition=Available",
		"--namespace", "cert-manager",
		"--timeout", "5m",
	)
	if _, err := Run(cmd); err != nil {
		return err
	}

	// Wait for the webhook to be able to serve TLS requests.
	// The deployment may be Available before the webhook CA is fully provisioned.
	return waitForCertManagerWebhookReady()
}

// waitForCertManagerWebhookReady polls cert-manager by creating and deleting
// a self-signed Issuer until the webhook accepts the request (TLS ready).
func waitForCertManagerWebhookReady() error {
	issuerYAML := `apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: cert-manager-readiness-probe
  namespace: cert-manager
spec:
  selfSigned: {}`

	for i := range 30 {
		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(issuerYAML)
		if _, err := Run(cmd); err == nil {
			// Webhook accepted the request — clean up the probe resource.
			cleanup := exec.Command("kubectl", "delete", "issuer",
				"cert-manager-readiness-probe", "-n", "cert-manager", "--ignore-not-found")
			_, _ = Run(cleanup)
			return nil
		}
		_, _ = fmt.Fprintf(GinkgoWriter, "cert-manager webhook not ready yet, retrying (%d/30)...\n", i+1)
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("cert-manager webhook did not become ready within 60s")
}

// IsCertManagerCRDsInstalled checks if any Cert Manager CRDs are installed
// by verifying the existence of key CRDs related to Cert Manager.
func IsCertManagerCRDsInstalled() bool {
	// List of common Cert Manager CRDs
	certManagerCRDs := []string{
		"certificates.cert-manager.io",
		"issuers.cert-manager.io",
		"clusterissuers.cert-manager.io",
		"certificaterequests.cert-manager.io",
		"orders.acme.cert-manager.io",
		"challenges.acme.cert-manager.io",
	}

	// Execute the kubectl command to get all CRDs
	cmd := exec.Command("kubectl", "get", "crds")
	output, err := Run(cmd)
	if err != nil {
		return false
	}

	// Check if any of the Cert Manager CRDs are present
	crdList := GetNonEmptyLines(output)
	for _, crd := range certManagerCRDs {
		for _, line := range crdList {
			if strings.Contains(line, crd) {
				return true
			}
		}
	}

	return false
}

// LoadImageToKindClusterWithName loads a local container image to the kind cluster.
// When using Podman (detected via CONTAINER_TOOL=podman or KIND_PROVIDER=podman),
// it uses podman save + kind load image-archive because kind load docker-image
// has compatibility issues with Podman on macOS.
func LoadImageToKindClusterWithName(name string) error {
	cluster := defaultKindCluster
	if v, ok := os.LookupEnv("KIND_CLUSTER"); ok {
		cluster = v
	}
	kindBinary := defaultKindBinary
	if v, ok := os.LookupEnv("KIND"); ok {
		kindBinary = v
	}

	usePodman := os.Getenv("CONTAINER_TOOL") == podmanRuntime ||
		os.Getenv("KIND_PROVIDER") == podmanRuntime ||
		os.Getenv("KIND_EXPERIMENTAL_PROVIDER") == podmanRuntime ||
		isPodmanOnlyEnvironment()

	// When using Podman, kind load docker-image may fail to find images.
	// Use podman save + kind load image-archive as a workaround.
	if usePodman {
		containerTool := os.Getenv("CONTAINER_TOOL")
		if containerTool == "" {
			containerTool = podmanRuntime
		}

		f, err := os.CreateTemp("", "kind-image-*.tar")
		if err != nil {
			return fmt.Errorf("creating temp archive: %w", err)
		}
		archivePath := f.Name()
		_ = f.Close()
		// Remove the empty file so podman save can create it fresh
		// (some versions refuse to overwrite an existing file).
		_ = os.Remove(archivePath)
		defer func() { _ = os.Remove(archivePath) }()

		saveArgs := []string{"save"}
		if containerTool == podmanRuntime {
			saveArgs = append(saveArgs, "--format", "docker-archive")
		}
		saveArgs = append(saveArgs, name, "-o", archivePath)
		cmd := exec.Command(containerTool, saveArgs...)
		if _, err := Run(cmd); err != nil {
			return fmt.Errorf("%s save: %w", containerTool, err)
		}
		cmd = exec.Command(kindBinary, "load", "image-archive", archivePath, "--name", cluster)
		_, err = Run(cmd)
		return err
	}

	cmd := exec.Command(kindBinary, "load", "docker-image", name, "--name", cluster)
	_, err := Run(cmd)
	return err
}

// isPodmanOnlyEnvironment returns true when Podman is available but Docker is not
// found on the PATH. This avoids accidentally using the Podman code path on
// systems that have both Docker and Podman installed side-by-side.
func isPodmanOnlyEnvironment() bool {
	_, podmanErr := exec.LookPath(podmanRuntime)
	_, dockerErr := exec.LookPath("docker")
	// Use errors.Is for both sides so that permission errors or other
	// non-ErrNotFound failures are not confused with "binary not found".
	podmanNotFound := errors.Is(podmanErr, exec.ErrNotFound)
	dockerNotFound := errors.Is(dockerErr, exec.ErrNotFound)
	return !podmanNotFound && dockerNotFound
}

// GetNonEmptyLines converts given command output string into individual objects
// according to line breakers, and ignores the empty elements in it.
func GetNonEmptyLines(output string) []string {
	var res []string
	elements := strings.SplitSeq(output, "\n")
	for element := range elements {
		if element != "" {
			res = append(res, element)
		}
	}

	return res
}

// GetProjectDir will return the directory where the project is
func GetProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return wd, fmt.Errorf("failed to get current working directory: %w", err)
	}
	wd = strings.ReplaceAll(wd, "/test/e2e", "")
	return wd, nil
}
