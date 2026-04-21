package podutil

import (
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"

	v1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/storage"
)

const (
	// AerospikeContainerName aliases the canonical constant from api/v1alpha1.
	AerospikeContainerName = v1alpha1.AerospikeContainerName
	InitContainerName      = "aerospike-init"

	// Port constants alias the canonical values from api/v1alpha1
	// so existing callers (e.g. podutil.ServicePort) continue to work.
	ServicePort         = v1alpha1.DefaultServicePort
	FabricPort          = v1alpha1.DefaultFabricPort
	HeartbeatPort       = v1alpha1.DefaultHeartbeatPort
	InfoPort      int32 = 3003

	configMapVolumeMountPath = "/configmap"
	aerospikeConfigPath      = "/etc/aerospike"
)

// PreStopSleepSeconds is the number of seconds the PreStop lifecycle hook sleeps
// before the container receives SIGTERM. This gives in-flight requests time to
// complete and allows Kubernetes to update endpoints/iptables so new traffic is
// no longer routed to the terminating pod.
const PreStopSleepSeconds = 15

// BuildAerospikeContainer creates the main Aerospike server container spec.
func BuildAerospikeContainer(cluster *v1alpha1.AerospikeCluster, volumeMounts []corev1.VolumeMount) corev1.Container {
	c := corev1.Container{
		Name:  AerospikeContainerName,
		Image: cluster.Spec.Image,
		Command: []string{
			"/usr/bin/asd",
			"--foreground",
		},
		Ports: []corev1.ContainerPort{
			{Name: "service", ContainerPort: ServicePort, Protocol: corev1.ProtocolTCP},
			{Name: "fabric", ContainerPort: FabricPort, Protocol: corev1.ProtocolTCP},
			{Name: "heartbeat", ContainerPort: HeartbeatPort, Protocol: corev1.ProtocolTCP},
			{Name: "info", ContainerPort: InfoPort, Protocol: corev1.ProtocolTCP},
		},
		VolumeMounts: volumeMounts,
		Lifecycle: &corev1.Lifecycle{
			PreStop: &corev1.LifecycleHandler{
				Exec: &corev1.ExecAction{
					Command: []string{"sleep", fmt.Sprintf("%d", PreStopSleepSeconds)},
				},
			},
		},
		ReadinessProbe: buildReadinessProbe(),
		LivenessProbe:  buildLivenessProbe(),
	}

	if cluster.Spec.PodSpec != nil && cluster.Spec.PodSpec.AerospikeContainerSpec != nil {
		spec := cluster.Spec.PodSpec.AerospikeContainerSpec
		if spec.Resources != nil {
			c.Resources = *spec.Resources
		}
		if spec.SecurityContext != nil {
			c.SecurityContext = spec.SecurityContext
		}
	}

	return c
}

// buildLivenessProbe creates an exec-based liveness probe that verifies the
// Aerospike server is responsive by querying the build version via asinfo.
// This is more reliable than a TCP socket check because it confirms the
// Aerospike process is actively handling info protocol requests.
func buildLivenessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"/bin/sh", "-c",
					fmt.Sprintf("/usr/bin/asinfo -v 'build' -h 127.0.0.1 -p %d", ServicePort),
				},
			},
		},
		InitialDelaySeconds: 30,
		PeriodSeconds:       30,
		TimeoutSeconds:      5,
		FailureThreshold:    5,
	}
}

// buildReadinessProbe creates an exec-based readiness probe that verifies the
// Aerospike server is ready to accept client requests by checking that
// cluster_size is reported in statistics. This ensures the node has joined
// the cluster and is serving data, not just listening on a TCP port.
func buildReadinessProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: []string{
					"/bin/sh", "-c",
					fmt.Sprintf("/usr/bin/asinfo -v 'statistics' -h 127.0.0.1 -p %d 2>&1 | grep -q 'cluster_size'", ServicePort),
				},
			},
		},
		InitialDelaySeconds: 15,
		PeriodSeconds:       10,
		TimeoutSeconds:      5,
		FailureThreshold:    3,
	}
}

// BuildInitContainer creates the init container that runs the aerospike-init.sh script
// to copy and process configuration files (placeholder substitution, volume initialization).
// dirtyVolumes is the list of volume names that need wiping (from pod status DirtyVolumes).
func BuildInitContainer(
	cluster *v1alpha1.AerospikeCluster,
	configMapName string,
	storageSpec *v1alpha1.AerospikeStorageSpec,
	volumeMounts []corev1.VolumeMount,
	dirtyVolumes []string,
) corev1.Container {
	// Ensure the init container has mounts for both the configmap source
	// and the aerospike config destination.
	initMounts := make([]corev1.VolumeMount, 0, 2+len(volumeMounts))
	initMounts = append(initMounts,
		corev1.VolumeMount{
			Name:      configMapVolumeName,
			MountPath: configMapVolumeMountPath,
			ReadOnly:  true,
		},
		corev1.VolumeMount{
			Name:      aerospikeConfigVolumeName,
			MountPath: aerospikeConfigPath,
		},
	)
	initMounts = append(initMounts, volumeMounts...)

	env := buildInitEnvVars()

	// Inject external service type and namespace for init container to resolve
	// external addresses from per-pod LoadBalancer/NodePort services.
	if cluster.Spec.PodService != nil && cluster.Spec.PodService.ServiceType != "" &&
		cluster.Spec.PodService.ServiceType != "ClusterIP" {
		env = append(env,
			corev1.EnvVar{
				Name:  "EXTERNAL_SERVICE_TYPE",
				Value: cluster.Spec.PodService.ServiceType,
			},
			corev1.EnvVar{
				Name: "POD_NAMESPACE",
				ValueFrom: &corev1.EnvVarSource{
					FieldRef: &corev1.ObjectFieldSelector{
						FieldPath: "metadata.namespace",
					},
				},
			},
		)
	}

	if wipeVolumes := buildWipeVolumesEnv(storageSpec, dirtyVolumes); wipeVolumes != "" {
		env = append(env, corev1.EnvVar{
			Name:  "WIPE_VOLUMES",
			Value: wipeVolumes,
		})
	}
	if initVolumes := buildInitVolumesEnv(storageSpec); initVolumes != "" {
		env = append(env, corev1.EnvVar{
			Name:  "INIT_VOLUMES",
			Value: initVolumes,
		})
	}

	return corev1.Container{
		Name:  InitContainerName,
		Image: cluster.Spec.Image,
		Command: []string{
			"bash", "/configmap/aerospike-init.sh",
		},
		Env:          env,
		VolumeMounts: initMounts,
	}
}

// buildInitEnvVars returns the Downward API environment variables for the init container.
func buildInitEnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name: "POD_IP",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.podIP",
				},
			},
		},
		{
			Name: "POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name: "NODE_IP",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.hostIP",
				},
			},
		},
	}
}

// buildInitVolumesEnv converts storage spec InitMethod fields into an INIT_VOLUMES
// environment variable value. Format: "method1:path1,method2:path2"
// Uses policy resolution: per-volume InitMethod > global policy > "none".
func buildInitVolumesEnv(storageSpec *v1alpha1.AerospikeStorageSpec) string {
	if storageSpec == nil {
		return ""
	}

	var parts []string
	for i := range storageSpec.Volumes {
		vol := &storageSpec.Volumes[i]
		method := storage.ResolveInitMethod(vol, storageSpec)
		if method == "" || method == v1alpha1.VolumeInitMethodNone {
			continue
		}
		if vol.Aerospike == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:%s", method, vol.Aerospike.Path))
	}

	return strings.Join(parts, ",")
}

// buildWipeVolumesEnv builds the WIPE_VOLUMES environment variable for dirty volumes.
// Only volumes that are in the dirtyVolumes list and have a resolved wipe method
// (per-volume or global policy) are included. Format: "method1:path1,method2:path2"
func buildWipeVolumesEnv(storageSpec *v1alpha1.AerospikeStorageSpec, dirtyVolumes []string) string {
	if storageSpec == nil || len(dirtyVolumes) == 0 {
		return ""
	}

	var parts []string
	for i := range storageSpec.Volumes {
		vol := &storageSpec.Volumes[i]
		if !slices.Contains(dirtyVolumes, vol.Name) {
			continue
		}
		method := storage.ResolveWipeMethod(vol, storageSpec)
		if method == "" || method == v1alpha1.VolumeWipeMethodNone {
			continue
		}
		if vol.Aerospike == nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:%s", method, vol.Aerospike.Path))
	}

	return strings.Join(parts, ",")
}
