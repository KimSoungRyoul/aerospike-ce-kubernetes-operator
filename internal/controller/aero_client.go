package controller

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	aero "github.com/aerospike/aerospike-client-go/v8"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	ackoerrors "github.com/ksr/aerospike-ce-kubernetes-operator/internal/errors"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/podutil"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/utils"
)

const (
	aeroClientTimeout = 30 * time.Second
	aeroLoginTimeout  = 10 * time.Second
	aeroInfoTimeout   = 10 * time.Second
	defaultAeroPort   = 3000

	// quiesceTimeout is the maximum time to wait for a quiesce command to complete.
	quiesceTimeout = 15 * time.Second
)

// getServicePort returns the configured Aerospike service port from the cluster
// config, falling back to the default port.
func getServicePort(cluster *ackov1alpha1.AerospikeCluster) int {
	if cluster.Spec.AerospikeConfig != nil {
		if netCfg, ok := cluster.Spec.AerospikeConfig.Value["network"].(map[string]any); ok {
			if svcCfg, ok := netCfg["service"].(map[string]any); ok {
				if port, ok := svcCfg["port"]; ok {
					return utils.IntFromAny(port, defaultAeroPort)
				}
			}
		}
	}
	return defaultAeroPort
}

// getAerospikeClient creates an Aerospike client connected to the cluster.
func (r *AerospikeClusterReconciler) getAerospikeClient(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) (*aero.Client, error) {
	log := logf.FromContext(ctx)

	serviceName := utils.HeadlessServiceName(cluster.Name)
	host := fmt.Sprintf("%s.%s.svc.cluster.local", serviceName, cluster.Namespace)
	port := getServicePort(cluster)

	policy := aero.NewClientPolicy()
	policy.Timeout = aeroClientTimeout
	policy.LoginTimeout = aeroLoginTimeout

	// If ACL is enabled, find the admin user dynamically by roles.
	if adminUser := utils.FindAdminUser(cluster.Spec.AerospikeAccessControl); adminUser != nil {
		password, err := r.getPasswordFromSecret(ctx, cluster.Namespace, adminUser.SecretName)
		if err != nil {
			return nil, fmt.Errorf("getting admin password: %w", err)
		}
		policy.User = adminUser.Name
		policy.Password = password
	}

	log.Info("Connecting to Aerospike cluster", "host", host, "port", port)
	client, err := aero.NewClientWithPolicy(policy, host, port)
	if err != nil {
		return nil, fmt.Errorf("connecting to Aerospike: %w", err)
	}

	return client, nil
}

// getAerospikeClientWithRetry wraps getAerospikeClient with retry logic using
// exponential backoff (2s, 4s, 8s). This is useful during initial deployment
// when DNS may not yet be resolving for the headless service.
//
// Permanent validation errors (e.g. a missing admin Secret or a Secret without
// a "password" key) are returned immediately: they will never self-heal, so
// retrying just burns ~14s of backoff sleep on every reconcile and delays the
// circuit breaker from surfacing the permanent error to the user.
func (r *AerospikeClusterReconciler) getAerospikeClientWithRetry(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
) (*aero.Client, error) {
	log := logf.FromContext(ctx)

	const maxRetries = 3
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt)) * time.Second // 2s, 4s, 8s
			log.V(1).Info("Retrying Aerospike client connection",
				"attempt", attempt, "maxRetries", maxRetries, "backoff", backoff)

			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, fmt.Errorf("context cancelled while retrying Aerospike connection: %w", ctx.Err())
			case <-timer.C:
			}
		}

		client, err := r.getAerospikeClient(ctx, cluster)
		if err == nil {
			return client, nil
		}
		lastErr = err
		// Don't burn backoff cycles on errors that cannot self-heal.
		if ackoerrors.IsValidation(err) {
			return nil, err
		}
	}

	return nil, fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// closeAerospikeClient safely closes an Aerospike client.
func closeAerospikeClient(client *aero.Client) {
	if client != nil {
		client.Close()
	}
}

// getPasswordFromSecret reads a password from a Kubernetes Secret.
func (r *AerospikeClusterReconciler) getPasswordFromSecret(
	ctx context.Context,
	namespace, secretName string,
) (string, error) {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: namespace}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return "", ackoerrors.NewValidationf("secret %s not found", secretName)
		}
		return "", fmt.Errorf("getting secret %s: %w", secretName, err)
	}

	password, ok := secret.Data["password"]
	if !ok {
		return "", ackoerrors.NewValidationf("secret %s does not have 'password' key", secretName)
	}

	return string(password), nil
}

// quiesceNode sends the "quiesce:" command to an Aerospike node via pod exec,
// so the node gracefully stops accepting new transactions before being removed.
// The command is executed inside the Aerospike container using asinfo.
// This is a best-effort operation: the caller should proceed with pod deletion
// even if quiesce fails (e.g., node is already down, asinfo not available).
func (r *AerospikeClusterReconciler) quiesceNode(
	ctx context.Context,
	pod *corev1.Pod,
	cluster *ackov1alpha1.AerospikeCluster,
) error {
	log := logf.FromContext(ctx)

	if r.RestConfig == nil {
		return fmt.Errorf("RestConfig not available for exec")
	}

	clientset, err := r.getOrCreateKubeClientset()
	if err != nil {
		return fmt.Errorf("creating kubernetes clientset for quiesce: %w", err)
	}

	port := getServicePort(cluster)

	// Resolve admin password for ACL-enabled clusters. asinfo requires both
	// -U and -P together; passing -U alone causes asinfo to prompt for a
	// password on stdin and hang/fail without a TTY.
	var adminPassword string
	if adminUser := utils.FindAdminUser(cluster.Spec.AerospikeAccessControl); adminUser != nil {
		pw, err := r.getPasswordFromSecret(ctx, cluster.Namespace, adminUser.SecretName)
		if err != nil {
			return fmt.Errorf("getting admin password for quiesce: %w", err)
		}
		adminPassword = pw
	}

	// Build the asinfo command. If ACL is enabled, include auth flags.
	cmd := buildQuiesceCommand(cluster, port, adminPassword)

	// Apply quiesce-specific timeout to prevent blocking pod deletion indefinitely.
	execCtx, cancel := context.WithTimeout(ctx, quiesceTimeout)
	defer cancel()

	req := clientset.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Namespace(pod.Namespace).
		Name(pod.Name).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: podutil.AerospikeContainerName,
			Command:   cmd,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(r.RestConfig, "POST", req.URL())
	if err != nil {
		return fmt.Errorf("creating SPDY executor for quiesce: %w", err)
	}

	var stdout, stderr bytes.Buffer
	if err := exec.StreamWithContext(execCtx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	}); err != nil {
		return fmt.Errorf("executing quiesce command: %w (stderr: %s)", err, stderr.String())
	}

	output := strings.TrimSpace(stdout.String())
	log.Info("Quiesce command completed", "pod", pod.Name, "output", output)

	// Aerospike returns "ok" on success. Any other response (including empty) indicates a problem.
	if output != "ok" {
		return fmt.Errorf("quiesce returned unexpected response: %q", output)
	}

	return nil
}

// buildQuiesceCommand constructs the asinfo command to quiesce a node.
// If ACL is enabled, both -U and -P authentication flags are included.
// Passing -U without -P would cause asinfo to prompt for a password on stdin
// and hang/fail without a TTY, so the caller must resolve adminPassword from
// the same Secret used by getPasswordFromSecret. An empty adminPassword with
// an admin user present results in -U only (preserved for callers that need
// to inspect the command shape, e.g. tests), but the production caller
// (quiesceNode) always supplies it.
func buildQuiesceCommand(cluster *ackov1alpha1.AerospikeCluster, port int, adminPassword string) []string {
	cmd := []string{
		"asinfo",
		"-v", "quiesce:",
		"-h", "localhost",
		"-p", fmt.Sprintf("%d", port),
	}

	// Add authentication flags if ACL is enabled.
	if adminUser := utils.FindAdminUser(cluster.Spec.AerospikeAccessControl); adminUser != nil {
		cmd = append(cmd, "-U", adminUser.Name)
		if adminPassword != "" {
			cmd = append(cmd, "-P", adminPassword)
		}
	}

	return cmd
}
