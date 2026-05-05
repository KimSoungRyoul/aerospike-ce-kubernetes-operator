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

package utils

import (
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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

	keycloakNamespace      = "keycloak"
	keycloakReleaseName    = "keycloak"
	keycloakRealmConfigMap = "acko-realm"
	keycloakChartRef       = "bitnami/keycloak"
	keycloakRepoName       = "bitnami"
	keycloakRepoURL        = "https://charts.bitnami.com/bitnami"
)

// ackoRealmJSON is the static Keycloak realm export used by the local/e2e
// IdP. Bitnami's keycloak-config-cli imports it on startup. See
// scripts/keycloak/acko-realm.json (Stream D contract C-5).
//
//go:embed testdata/acko-realm.json
var ackoRealmJSON []byte

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

// InstallKeycloak installs the bitnami/keycloak helm chart with the acko realm
// preloaded via keycloak-config-cli. Idempotent: re-running upgrades the
// release. Designed to mirror the Makefile `install-keycloak` target so e2e
// tests and `make run-local` behave identically.
func InstallKeycloak() error {
	if _, err := exec.LookPath("helm"); err != nil {
		return fmt.Errorf("helm binary not found on PATH: %w", err)
	}

	// helm repo add (ignore "already exists" errors)
	cmd := exec.Command("helm", "repo", "add", keycloakRepoName, keycloakRepoURL)
	if out, err := cmd.CombinedOutput(); err != nil &&
		!strings.Contains(string(out), "already exists") {
		return fmt.Errorf("helm repo add: %w (%s)", err, string(out))
	}
	cmd = exec.Command("helm", "repo", "update", keycloakRepoName)
	if _, err := Run(cmd); err != nil {
		return fmt.Errorf("helm repo update: %w", err)
	}

	// Ensure namespace
	if err := applyDryRunResource(
		[]string{"kubectl", "create", "namespace", keycloakNamespace, "--dry-run=client", "-o", "yaml"},
	); err != nil {
		return fmt.Errorf("create namespace %q: %w", keycloakNamespace, err)
	}

	// Write realm JSON to a temp file then turn into ConfigMap. We use
	// `--from-file=acko-realm.json=<path>` so the ConfigMap key is exactly
	// `acko-realm.json` regardless of the temp filename.
	tmp, err := os.CreateTemp("", "acko-realm-*.json")
	if err != nil {
		return fmt.Errorf("temp realm file: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(ackoRealmJSON); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write realm json: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close realm json: %w", err)
	}

	if err := applyDryRunResource([]string{
		"kubectl", "create", "configmap", keycloakRealmConfigMap,
		"--from-file=acko-realm.json=" + tmp.Name(),
		"-n", keycloakNamespace,
		"--dry-run=client", "-o", "yaml",
	}); err != nil {
		return fmt.Errorf("apply realm configmap: %w", err)
	}

	// helm upgrade --install keycloak
	helmArgs := []string{
		"upgrade", "--install", keycloakReleaseName, keycloakChartRef,
		"--namespace", keycloakNamespace,
		"--set", "auth.adminUser=admin",
		"--set", "auth.adminPassword=admin",
		"--set", "keycloakConfigCli.enabled=true",
		"--set", "keycloakConfigCli.existingConfigmap=" + keycloakRealmConfigMap,
		"--set", "service.type=ClusterIP",
		"--set", "proxy=edge",
		"--wait", "--timeout", "5m",
	}
	cmd = exec.Command("helm", helmArgs...)
	if _, err := Run(cmd); err != nil {
		return fmt.Errorf("helm install keycloak: %w", err)
	}

	cmd = exec.Command("kubectl", "wait", "deployment.apps/keycloak",
		"--for", "condition=Available",
		"--namespace", keycloakNamespace,
		"--timeout", "5m",
	)
	if _, err := Run(cmd); err != nil {
		return fmt.Errorf("wait keycloak deployment: %w", err)
	}

	return WaitForKeycloakReady()
}

// UninstallKeycloak removes the local keycloak release and its namespace.
// Best-effort; intended for AfterSuite cleanup paths.
func UninstallKeycloak() {
	cmd := exec.Command("helm", "uninstall", keycloakReleaseName, "-n", keycloakNamespace)
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
	cmd = exec.Command("kubectl", "delete", "namespace", keycloakNamespace, "--ignore-not-found")
	if _, err := Run(cmd); err != nil {
		warnError(err)
	}
}

// WaitForKeycloakReady polls Keycloak's well-known OIDC endpoint until it
// returns HTTP 200. Uses an in-cluster `kubectl exec` so this works whether
// or not a port-forward is set up — important because BeforeSuite runs
// before any test fixtures.
func WaitForKeycloakReady() error {
	const wellKnown = "http://localhost/realms/acko/.well-known/openid-configuration"
	for i := range 60 {
		cmd := exec.Command("kubectl", "exec", "-n", keycloakNamespace,
			"deployment/keycloak", "--",
			"curl", "-sS", "-o", "/dev/null", "-w", "%{http_code}", wellKnown)
		out, err := cmd.CombinedOutput()
		if err == nil && strings.TrimSpace(string(out)) == "200" {
			return nil
		}
		_, _ = fmt.Fprintf(GinkgoWriter,
			"keycloak realm not ready yet, retrying (%d/60): rc=%v out=%q\n",
			i+1, err, strings.TrimSpace(string(out)))
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("keycloak realm 'acko' did not become ready within 120s")
}

// PingKeycloakHTTP is exposed for any caller that already has a port-forward
// or NodePort and just needs to verify the realm is up over HTTP. Returns nil
// on the first 200 within 60s.
func PingKeycloakHTTP(baseURL string) error {
	url := strings.TrimRight(baseURL, "/") + "/realms/acko/.well-known/openid-configuration"
	client := &http.Client{Timeout: 5 * time.Second}
	for i := range 30 {
		resp, err := client.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		_, _ = fmt.Fprintf(GinkgoWriter, "keycloak HTTP %s not 200 yet (%d/30)\n", url, i+1)
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("keycloak %s did not return 200 within 60s", url)
}

// applyDryRunResource runs the given `kubectl create ... --dry-run=client -o yaml`
// command, then pipes the resulting YAML into `kubectl apply -f -`. This is
// the idempotent apply pattern used throughout the e2e suite.
func applyDryRunResource(generateCmd []string) error {
	gen := exec.Command(generateCmd[0], generateCmd[1:]...) // nolint:gosec
	yamlOut, err := gen.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %w (%s)", strings.Join(generateCmd, " "), err, string(yamlOut))
	}
	apply := exec.Command("kubectl", "apply", "-f", "-")
	apply.Stdin = strings.NewReader(string(yamlOut))
	if _, err := Run(apply); err != nil {
		return fmt.Errorf("kubectl apply (%s): %w", generateCmd[2], err)
	}
	return nil
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

	containerToolEnv, containerToolSet := os.LookupEnv("CONTAINER_TOOL")
	usePodman := containerToolEnv == podmanRuntime ||
		os.Getenv("KIND_PROVIDER") == podmanRuntime ||
		os.Getenv("KIND_EXPERIMENTAL_PROVIDER") == podmanRuntime ||
		(!containerToolSet && isPodmanOnlyEnvironment())

	// When using Podman, kind load docker-image may fail to find images.
	// Use podman save + kind load image-archive as a workaround.
	if usePodman {
		fmt.Fprintf(os.Stderr, "[info] using Podman code path for image loading (cluster=%q, image=%q)\n", cluster, name)
		containerTool := containerToolEnv
		if containerTool == "" {
			containerTool = podmanRuntime
		}

		tmpDir, err := os.MkdirTemp("", "kind-image-*")
		if err != nil {
			return fmt.Errorf("creating temp directory: %w", err)
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()
		archivePath := filepath.Join(tmpDir, "image.tar")

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
	podmanFound := podmanErr == nil
	dockerFound := dockerErr == nil
	return podmanFound && !dockerFound
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
	wd = strings.TrimSuffix(wd, "/test/e2e")
	return wd, nil
}
