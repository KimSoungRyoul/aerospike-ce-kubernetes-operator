package podutil

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	v1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/storage"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/utils"
)

const (
	configMapVolumeName        = "aerospike-config-map"
	aerospikeConfigVolumeName  = "aerospike-config"
	defaultTerminationGraceSec = int64(60)

	// AerospikeReadinessGateConditionType is the custom pod condition type injected
	// when spec.podSpec.readinessGateEnabled=true. The operator patches this condition
	// to True once Aerospike has joined the cluster mesh and all data migrations are complete.
	AerospikeReadinessGateConditionType corev1.PodConditionType = "acko.io/aerospike-ready"
)

// BuildPodTemplateSpec builds the complete PodTemplateSpec for a StatefulSet
// managing Aerospike pods in the given rack.
func BuildPodTemplateSpec(
	cluster *v1alpha1.AerospikeCluster,
	rack *v1alpha1.Rack,
	rackID int,
	configMapName string,
	configHash string,
) corev1.PodTemplateSpec {
	// Determine the effective storage spec (rack override or cluster-level).
	storageSpec := cluster.Spec.Storage
	if rack != nil && rack.Storage != nil {
		storageSpec = rack.Storage
	}

	// Build volumes and mounts from storage spec.
	volumes, aerospikeMounts := storage.BuildVolumes(storageSpec)

	// Add the configmap volume (source) and the writable aerospike-config volume.
	volumes = append(volumes,
		corev1.Volume{
			Name: configMapVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMapName,
					},
				},
			},
		},
		corev1.Volume{
			Name: aerospikeConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	)

	// Ensure the aerospike container has the config volume mounted.
	aerospikeMounts = append(aerospikeMounts, corev1.VolumeMount{
		Name:      aerospikeConfigVolumeName,
		MountPath: aerospikeConfigPath,
	})

	// Labels.
	labels := utils.LabelsForRack(cluster.Name, rackID)

	// Annotations.
	annotations := map[string]string{
		utils.ConfigHashAnnotation: configHash,
	}

	// Inject bandwidth annotations if configured.
	if cluster.Spec.BandwidthConfig != nil {
		if cluster.Spec.BandwidthConfig.Ingress != "" {
			annotations["kubernetes.io/ingress-bandwidth"] = cluster.Spec.BandwidthConfig.Ingress
		}
		if cluster.Spec.BandwidthConfig.Egress != "" {
			annotations["kubernetes.io/egress-bandwidth"] = cluster.Spec.BandwidthConfig.Egress
		}
	}

	// Merge user-provided pod metadata.
	if cluster.Spec.PodSpec != nil && cluster.Spec.PodSpec.Metadata != nil {
		maps.Copy(labels, cluster.Spec.PodSpec.Metadata.Labels)
		maps.Copy(annotations, cluster.Spec.PodSpec.Metadata.Annotations)
	}

	// Build containers.
	initVolumeMounts := storage.VolumeMountsForContainer(storageSpec, InitContainerName, false)
	// DirtyVolumes tracking is implemented in reconciler_restart.go (markDirtyVolumes)
	// and reconciler_status.go (preserve/clear on ready). The init container receives
	// nil here because StatefulSet applies a uniform pod template; per-pod WIPE_VOLUMES
	// injection requires an annotation-based mechanism (future work).
	initContainer := BuildInitContainer(cluster, configMapName, storageSpec, initVolumeMounts, nil)
	aerospikeContainer := BuildAerospikeContainer(cluster, aerospikeMounts)

	// Init containers: operator init + user-defined.
	initContainers := []corev1.Container{initContainer}

	// Sidecars.
	var sidecars []corev1.Container

	// Inject Prometheus exporter sidecar if monitoring is enabled.
	if cluster.Spec.Monitoring != nil && cluster.Spec.Monitoring.Enabled {
		sidecars = append(sidecars, buildExporterSidecar(cluster.Spec.Monitoring, cluster.Spec.AerospikeAccessControl))
	}

	if cluster.Spec.PodSpec != nil {
		for _, ic := range cluster.Spec.PodSpec.InitContainers {
			mounts := storage.VolumeMountsForContainer(storageSpec, ic.Name, false)
			ic.VolumeMounts = append(ic.VolumeMounts, mounts...)
			initContainers = append(initContainers, ic)
		}

		for _, sc := range cluster.Spec.PodSpec.Sidecars {
			mounts := storage.VolumeMountsForContainer(storageSpec, sc.Name, true)
			sc.VolumeMounts = append(sc.VolumeMounts, mounts...)
			sidecars = append(sidecars, sc)
		}
	}

	allContainers := make([]corev1.Container, 0, 1+len(sidecars))
	allContainers = append(allContainers, aerospikeContainer)
	allContainers = append(allContainers, sidecars...)

	// Termination grace period.
	terminationGrace := defaultTerminationGraceSec
	if cluster.Spec.PodSpec != nil && cluster.Spec.PodSpec.TerminationGracePeriodSeconds != nil {
		terminationGrace = *cluster.Spec.PodSpec.TerminationGracePeriodSeconds
	}

	podSpec := corev1.PodSpec{
		InitContainers:                initContainers,
		Containers:                    allContainers,
		Volumes:                       volumes,
		TerminationGracePeriodSeconds: &terminationGrace,
	}

	// Pod-level settings from cluster spec.
	if cluster.Spec.PodSpec != nil {
		applyPodSpecSettings(&podSpec, cluster.Spec.PodSpec)
	}

	// Inject pod anti-affinity when multiPodPerHost is explicitly false.
	if shouldInjectAntiAffinity(cluster) {
		injectPodAntiAffinity(&podSpec, cluster.Name)
	}

	// Rack-level overrides. Apply zone/region/node affinity first, then
	// rack PodSpec overrides. This ensures rack affinity is not silently
	// skipped when rack.PodSpec is set.
	if rack != nil {
		applyRackAffinity(&podSpec, rack)
		if rack.PodSpec != nil {
			applyRackPodSpecOverrides(&podSpec, rack.PodSpec)
		}
	}

	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: podSpec,
	}
}

// applyPodSpecSettings applies cluster-level pod spec settings.
func applyPodSpecSettings(podSpec *corev1.PodSpec, spec *v1alpha1.AerospikePodSpec) {
	if spec.Affinity != nil {
		podSpec.Affinity = spec.Affinity
	}

	if len(spec.Tolerations) > 0 {
		podSpec.Tolerations = spec.Tolerations
	}

	if len(spec.NodeSelector) > 0 {
		podSpec.NodeSelector = spec.NodeSelector
	}

	if spec.SecurityContext != nil {
		podSpec.SecurityContext = spec.SecurityContext
	}

	if spec.ServiceAccountName != "" {
		podSpec.ServiceAccountName = spec.ServiceAccountName
	}

	if spec.DNSPolicy != "" {
		podSpec.DNSPolicy = spec.DNSPolicy
	}

	if spec.HostNetwork {
		podSpec.HostNetwork = true
	}

	if len(spec.ImagePullSecrets) > 0 {
		podSpec.ImagePullSecrets = spec.ImagePullSecrets
	}

	if spec.ReadinessGateEnabled != nil && *spec.ReadinessGateEnabled {
		podSpec.ReadinessGates = append(podSpec.ReadinessGates, corev1.PodReadinessGate{
			ConditionType: AerospikeReadinessGateConditionType,
		})
	}

	if len(spec.TopologySpreadConstraints) > 0 {
		podSpec.TopologySpreadConstraints = spec.TopologySpreadConstraints
	}

	if spec.PriorityClassName != "" {
		podSpec.PriorityClassName = spec.PriorityClassName
	}
}

// applyRackPodSpecOverrides overrides pod spec settings with rack-specific values.
func applyRackPodSpecOverrides(podSpec *corev1.PodSpec, rackPod *v1alpha1.RackPodSpec) {
	if rackPod.Affinity != nil {
		podSpec.Affinity = rackPod.Affinity
	}

	if len(rackPod.Tolerations) > 0 {
		podSpec.Tolerations = rackPod.Tolerations
	}

	if len(rackPod.NodeSelector) > 0 {
		podSpec.NodeSelector = rackPod.NodeSelector
	}
}

// applyRackAffinity sets node affinity based on rack zone/region/nodeName fields.
func applyRackAffinity(podSpec *corev1.PodSpec, rack *v1alpha1.Rack) {
	var terms []corev1.NodeSelectorRequirement

	if rack.Zone != "" {
		terms = append(terms, corev1.NodeSelectorRequirement{
			Key:      "topology.kubernetes.io/zone",
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{rack.Zone},
		})
	}

	if rack.Region != "" {
		terms = append(terms, corev1.NodeSelectorRequirement{
			Key:      "topology.kubernetes.io/region",
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{rack.Region},
		})
	}

	if rack.NodeName != "" {
		terms = append(terms, corev1.NodeSelectorRequirement{
			Key:      "kubernetes.io/hostname",
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{rack.NodeName},
		})
	}

	if rack.RackLabel != "" {
		terms = append(terms, corev1.NodeSelectorRequirement{
			Key:      "acko.io/rack",
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{rack.RackLabel},
		})
	}

	if len(terms) == 0 {
		return
	}

	if podSpec.Affinity == nil {
		podSpec.Affinity = &corev1.Affinity{}
	}

	podSpec.Affinity.NodeAffinity = &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{MatchExpressions: terms},
			},
		},
	}
}

// shouldInjectAntiAffinity returns true if pod anti-affinity should be injected
// to prevent multiple Aerospike pods from scheduling on the same node.
func shouldInjectAntiAffinity(cluster *v1alpha1.AerospikeCluster) bool {
	if cluster.Spec.PodSpec == nil {
		return false
	}

	multiPodPerHost := cluster.Spec.PodSpec.MultiPodPerHost
	if multiPodPerHost == nil {
		// nil means not explicitly set; only inject if hostNetwork is enabled
		// (webhook defaults multiPodPerHost=false for hostNetwork=true)
		return false
	}

	// Inject anti-affinity when multiPodPerHost is explicitly false
	return !*multiPodPerHost
}

// injectPodAntiAffinity adds a required pod anti-affinity rule to ensure
// at most one Aerospike pod per Kubernetes node. This appends to existing
// affinity rules rather than overwriting them.
func injectPodAntiAffinity(podSpec *corev1.PodSpec, clusterName string) {
	antiAffinityTerm := corev1.PodAffinityTerm{
		LabelSelector: &metav1.LabelSelector{
			MatchLabels: utils.SelectorLabelsForCluster(clusterName),
		},
		TopologyKey: "kubernetes.io/hostname",
	}

	if podSpec.Affinity == nil {
		podSpec.Affinity = &corev1.Affinity{}
	}
	if podSpec.Affinity.PodAntiAffinity == nil {
		podSpec.Affinity.PodAntiAffinity = &corev1.PodAntiAffinity{}
	}

	podSpec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution = append(
		podSpec.Affinity.PodAntiAffinity.RequiredDuringSchedulingIgnoredDuringExecution,
		antiAffinityTerm,
	)
}

// buildExporterSidecar creates the Prometheus exporter sidecar container with
// health probes, ACL authentication, custom env vars, and metric labels.
func buildExporterSidecar(
	monitoring *v1alpha1.AerospikeMonitoringSpec,
	acl *v1alpha1.AerospikeAccessControlSpec,
) corev1.Container {
	envVars := []corev1.EnvVar{
		{Name: "AS_HOST", Value: "localhost"},
		{Name: "AS_PORT", Value: fmt.Sprintf("%d", ServicePort)},
	}

	// Inject ACL credentials when access control is configured.
	if adminUser := utils.FindAdminUser(acl); adminUser != nil {
		envVars = append(envVars,
			corev1.EnvVar{Name: "AS_AUTH_USER", Value: adminUser.Name},
			corev1.EnvVar{
				Name: "AS_AUTH_PASSWORD",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: adminUser.SecretName,
						},
						Key: "password",
					},
				},
			},
			corev1.EnvVar{Name: "AS_AUTH_MODE", Value: "internal"},
		)
	}

	// Inject metric labels as METRIC_LABELS env var (sorted key=value pairs).
	if len(monitoring.MetricLabels) > 0 {
		keys := make([]string, 0, len(monitoring.MetricLabels))
		for k := range monitoring.MetricLabels {
			keys = append(keys, k)
		}
		slices.Sort(keys)

		pairs := make([]string, 0, len(keys))
		for _, k := range keys {
			pairs = append(pairs, fmt.Sprintf("%s=%s", k, monitoring.MetricLabels[k]))
		}
		envVars = append(envVars, corev1.EnvVar{
			Name:  "METRIC_LABELS",
			Value: strings.Join(pairs, ","),
		})
	}

	// Append user-provided env vars last so they can override defaults.
	envVars = append(envVars, monitoring.Env...)

	metricsProbe := &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/metrics",
				Port: intstr.FromInt32(monitoring.Port),
			},
		},
	}

	c := corev1.Container{
		Name:  "aerospike-prometheus-exporter",
		Image: monitoring.ExporterImage,
		Ports: []corev1.ContainerPort{
			{
				Name:          "metrics",
				ContainerPort: monitoring.Port,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: envVars,
		ReadinessProbe: &corev1.Probe{
			ProbeHandler:        metricsProbe.ProbeHandler,
			InitialDelaySeconds: 10,
			PeriodSeconds:       10,
			TimeoutSeconds:      5,
			FailureThreshold:    3,
		},
		LivenessProbe: &corev1.Probe{
			ProbeHandler:        metricsProbe.ProbeHandler,
			InitialDelaySeconds: 30,
			PeriodSeconds:       30,
			TimeoutSeconds:      5,
			FailureThreshold:    3,
		},
	}

	if monitoring.Resources != nil {
		c.Resources = *monitoring.Resources
	}

	return c
}

// PodNameForIndex returns the pod name for a given StatefulSet and ordinal index.
func PodNameForIndex(stsName string, index int) string {
	return fmt.Sprintf("%s-%d", stsName, index)
}
