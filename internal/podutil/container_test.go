package podutil

import (
	"fmt"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	v1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

const (
	testImage          = "aerospike:ce-8.1.1.1"
	deleteFilesDataEnv = "deleteFiles:/data"
)

func newTestCluster() *v1alpha1.AerospikeCluster {
	return &v1alpha1.AerospikeCluster{
		Spec: v1alpha1.AerospikeClusterSpec{
			Size:  3,
			Image: testImage,
		},
	}
}

func TestBuildInitContainer_UsesClusterImage(t *testing.T) {
	cluster := newTestCluster()

	c := BuildInitContainer(cluster, "my-config", nil, nil, nil)

	if c.Image != "aerospike:ce-8.1.1.1" {
		t.Errorf("expected image %q, got %q", "aerospike:ce-8.1.1.1", c.Image)
	}
}

func TestBuildInitContainer_RunsInitScript(t *testing.T) {
	cluster := newTestCluster()

	c := BuildInitContainer(cluster, "my-config", nil, nil, nil)

	if len(c.Command) != 2 || c.Command[0] != "bash" {
		t.Fatalf("expected command [bash /configmap/aerospike-init.sh], got %v", c.Command)
	}
	if !strings.Contains(c.Command[1], "aerospike-init.sh") {
		t.Errorf("expected command to reference aerospike-init.sh, got %v", c.Command)
	}
}

func TestBuildInitContainer_HasDownwardAPIEnvVars(t *testing.T) {
	cluster := newTestCluster()

	c := BuildInitContainer(cluster, "my-config", nil, nil, nil)

	envMap := make(map[string]*corev1.EnvVar)
	for i := range c.Env {
		envMap[c.Env[i].Name] = &c.Env[i]
	}

	// POD_IP
	if e, ok := envMap["POD_IP"]; !ok {
		t.Error("missing POD_IP env var")
	} else if e.ValueFrom == nil || e.ValueFrom.FieldRef == nil || e.ValueFrom.FieldRef.FieldPath != "status.podIP" {
		t.Errorf("POD_IP should use fieldRef status.podIP, got %+v", e.ValueFrom)
	}

	// POD_NAME
	if e, ok := envMap["POD_NAME"]; !ok {
		t.Error("missing POD_NAME env var")
	} else if e.ValueFrom == nil || e.ValueFrom.FieldRef == nil || e.ValueFrom.FieldRef.FieldPath != "metadata.name" {
		t.Errorf("POD_NAME should use fieldRef metadata.name, got %+v", e.ValueFrom)
	}

	// NODE_IP
	if e, ok := envMap["NODE_IP"]; !ok {
		t.Error("missing NODE_IP env var")
	} else if e.ValueFrom == nil || e.ValueFrom.FieldRef == nil || e.ValueFrom.FieldRef.FieldPath != "status.hostIP" {
		t.Errorf("NODE_IP should use fieldRef status.hostIP, got %+v", e.ValueFrom)
	}
}

func TestBuildInitContainer_HasConfigMapAndConfigMounts(t *testing.T) {
	cluster := newTestCluster()

	c := BuildInitContainer(cluster, "my-config", nil, nil, nil)

	mountNames := make(map[string]string)
	for _, m := range c.VolumeMounts {
		mountNames[m.Name] = m.MountPath
	}

	if path, ok := mountNames[configMapVolumeName]; !ok {
		t.Error("missing configmap volume mount")
	} else if path != configMapVolumeMountPath {
		t.Errorf("configmap mount path = %q, want %q", path, configMapVolumeMountPath)
	}

	if path, ok := mountNames[aerospikeConfigVolumeName]; !ok {
		t.Error("missing aerospike config volume mount")
	} else if path != aerospikeConfigPath {
		t.Errorf("config mount path = %q, want %q", path, aerospikeConfigPath)
	}
}

func TestBuildInitContainer_AppendsExtraVolumeMounts(t *testing.T) {
	cluster := newTestCluster()

	extraMounts := []corev1.VolumeMount{
		{Name: "data-vol", MountPath: "/opt/aerospike/data"},
	}

	c := BuildInitContainer(cluster, "my-config", nil, extraMounts, nil)

	found := false
	for _, m := range c.VolumeMounts {
		if m.Name == "data-vol" && m.MountPath == "/opt/aerospike/data" {
			found = true
			break
		}
	}
	if !found {
		t.Error("extra volume mount not found in init container")
	}
}

func TestBuildInitVolumesEnv_NilStorage(t *testing.T) {
	result := buildInitVolumesEnv(nil)
	if result != "" {
		t.Errorf("expected empty string for nil storage, got %q", result)
	}
}

func TestBuildInitVolumesEnv_NoInitMethods(t *testing.T) {
	storage := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name:       "data",
				InitMethod: v1alpha1.VolumeInitMethodNone,
				Aerospike:  &v1alpha1.AerospikeVolumeAttachment{Path: "/data"},
			},
		},
	}
	result := buildInitVolumesEnv(storage)
	if result != "" {
		t.Errorf("expected empty string for none init method, got %q", result)
	}
}

func TestBuildInitVolumesEnv_WithInitMethods(t *testing.T) {
	storage := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name:       "data",
				InitMethod: v1alpha1.VolumeInitMethodDeleteFiles,
				Aerospike:  &v1alpha1.AerospikeVolumeAttachment{Path: "/opt/aerospike/data"},
			},
			{
				Name:       "index",
				InitMethod: v1alpha1.VolumeInitMethodDD,
				Aerospike:  &v1alpha1.AerospikeVolumeAttachment{Path: "/opt/aerospike/index"},
			},
			{
				Name:       "no-init",
				InitMethod: v1alpha1.VolumeInitMethodNone,
				Aerospike:  &v1alpha1.AerospikeVolumeAttachment{Path: "/no-init"},
			},
		},
	}
	result := buildInitVolumesEnv(storage)
	expected := "deleteFiles:/opt/aerospike/data,dd:/opt/aerospike/index"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildInitVolumesEnv_SkipsVolumesWithoutAerospikePath(t *testing.T) {
	storage := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name:       "sidecar-only",
				InitMethod: v1alpha1.VolumeInitMethodDeleteFiles,
				// No Aerospike attachment
			},
		},
	}
	result := buildInitVolumesEnv(storage)
	if result != "" {
		t.Errorf("expected empty string for volume without aerospike path, got %q", result)
	}
}

func TestBuildInitContainer_INIT_VOLUMES_EnvVar(t *testing.T) {
	cluster := newTestCluster()
	storage := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name:       "data",
				InitMethod: v1alpha1.VolumeInitMethodDeleteFiles,
				Aerospike:  &v1alpha1.AerospikeVolumeAttachment{Path: "/data"},
			},
		},
	}

	c := BuildInitContainer(cluster, "my-config", storage, nil, nil)

	var initVolumes string
	for _, env := range c.Env {
		if env.Name == "INIT_VOLUMES" {
			initVolumes = env.Value
			break
		}
	}
	if initVolumes != deleteFilesDataEnv {
		t.Errorf("INIT_VOLUMES = %q, want %q", initVolumes, deleteFilesDataEnv)
	}
}

func TestBuildInitContainer_NoINIT_VOLUMES_WhenEmpty(t *testing.T) {
	cluster := newTestCluster()

	c := BuildInitContainer(cluster, "my-config", nil, nil, nil)

	for _, env := range c.Env {
		if env.Name == "INIT_VOLUMES" {
			t.Error("INIT_VOLUMES should not be present when no volumes need init")
		}
	}
}

// --- buildWipeVolumesEnv tests ---

func TestBuildWipeVolumesEnv_NilStorage(t *testing.T) {
	result := buildWipeVolumesEnv(nil, []string{"data"})
	if result != "" {
		t.Errorf("expected empty string for nil storage, got %q", result)
	}
}

func TestBuildWipeVolumesEnv_EmptyDirtyVolumes(t *testing.T) {
	storageSpec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name:       "data",
				WipeMethod: v1alpha1.VolumeWipeMethodDeleteFiles,
				Aerospike:  &v1alpha1.AerospikeVolumeAttachment{Path: "/data"},
			},
		},
	}
	result := buildWipeVolumesEnv(storageSpec, nil)
	if result != "" {
		t.Errorf("expected empty string for empty dirty volumes, got %q", result)
	}

	result = buildWipeVolumesEnv(storageSpec, []string{})
	if result != "" {
		t.Errorf("expected empty string for empty dirty volumes slice, got %q", result)
	}
}

func TestBuildWipeVolumesEnv_WithDirtyVolumes(t *testing.T) {
	storageSpec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name:       "data",
				WipeMethod: v1alpha1.VolumeWipeMethodDeleteFiles,
				Aerospike:  &v1alpha1.AerospikeVolumeAttachment{Path: "/opt/aerospike/data"},
			},
			{
				Name:       "index",
				WipeMethod: v1alpha1.VolumeWipeMethodDD,
				Aerospike:  &v1alpha1.AerospikeVolumeAttachment{Path: "/opt/aerospike/index"},
			},
		},
	}

	result := buildWipeVolumesEnv(storageSpec, []string{"data", "index"})
	expected := "deleteFiles:/opt/aerospike/data,dd:/opt/aerospike/index"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildWipeVolumesEnv_SkipsNonDirtyVolumes(t *testing.T) {
	storageSpec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name:       "data",
				WipeMethod: v1alpha1.VolumeWipeMethodDeleteFiles,
				Aerospike:  &v1alpha1.AerospikeVolumeAttachment{Path: "/data"},
			},
			{
				Name:       "clean-vol",
				WipeMethod: v1alpha1.VolumeWipeMethodDD,
				Aerospike:  &v1alpha1.AerospikeVolumeAttachment{Path: "/clean"},
			},
		},
	}

	// Only "data" is dirty
	result := buildWipeVolumesEnv(storageSpec, []string{"data"})
	expected := deleteFilesDataEnv
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildWipeVolumesEnv_SkipsNoneWipeMethod(t *testing.T) {
	storageSpec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name:       "data",
				WipeMethod: v1alpha1.VolumeWipeMethodNone,
				Aerospike:  &v1alpha1.AerospikeVolumeAttachment{Path: "/data"},
			},
		},
	}

	result := buildWipeVolumesEnv(storageSpec, []string{"data"})
	if result != "" {
		t.Errorf("expected empty string for none wipe method, got %q", result)
	}
}

func TestBuildWipeVolumesEnv_SkipsWithoutAerospikePath(t *testing.T) {
	storageSpec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name:       "sidecar-only",
				WipeMethod: v1alpha1.VolumeWipeMethodDeleteFiles,
				// No Aerospike attachment
			},
		},
	}

	result := buildWipeVolumesEnv(storageSpec, []string{"sidecar-only"})
	if result != "" {
		t.Errorf("expected empty string for volume without aerospike path, got %q", result)
	}
}

func TestBuildWipeVolumesEnv_WithGlobalPolicyFallback(t *testing.T) {
	storageSpec := &v1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			WipeMethod: v1alpha1.VolumeWipeMethodDeleteFiles,
		},
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				// No per-volume wipe method — should use global filesystem policy
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
				},
				Aerospike: &v1alpha1.AerospikeVolumeAttachment{Path: "/data"},
			},
		},
	}

	result := buildWipeVolumesEnv(storageSpec, []string{"data"})
	expected := deleteFilesDataEnv
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildWipeVolumesEnv_BlkdiscardWithHeaderCleanup(t *testing.T) {
	storageSpec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name:       "data",
				WipeMethod: v1alpha1.VolumeWipeMethodBlkdiscardWithHeaderCleanup,
				Aerospike:  &v1alpha1.AerospikeVolumeAttachment{Path: "/data"},
			},
		},
	}

	result := buildWipeVolumesEnv(storageSpec, []string{"data"})
	expected := "blkdiscardWithHeaderCleanup:/data"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildInitContainer_WIPE_VOLUMES_EnvVar(t *testing.T) {
	cluster := newTestCluster()
	storageSpec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name:       "data",
				WipeMethod: v1alpha1.VolumeWipeMethodDeleteFiles,
				Aerospike:  &v1alpha1.AerospikeVolumeAttachment{Path: "/data"},
			},
		},
	}

	c := BuildInitContainer(cluster, "my-config", storageSpec, nil, []string{"data"})

	var wipeVolumes string
	for _, env := range c.Env {
		if env.Name == "WIPE_VOLUMES" {
			wipeVolumes = env.Value
			break
		}
	}
	if wipeVolumes != deleteFilesDataEnv {
		t.Errorf("WIPE_VOLUMES = %q, want %q", wipeVolumes, deleteFilesDataEnv)
	}
}

func TestBuildInitContainer_NoWIPE_VOLUMES_WhenNoDirtyVolumes(t *testing.T) {
	cluster := newTestCluster()
	storageSpec := &v1alpha1.AerospikeStorageSpec{
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name:       "data",
				WipeMethod: v1alpha1.VolumeWipeMethodDeleteFiles,
				Aerospike:  &v1alpha1.AerospikeVolumeAttachment{Path: "/data"},
			},
		},
	}

	c := BuildInitContainer(cluster, "my-config", storageSpec, nil, nil)

	for _, env := range c.Env {
		if env.Name == "WIPE_VOLUMES" {
			t.Error("WIPE_VOLUMES should not be present when no dirty volumes")
		}
	}
}

func TestBuildInitVolumesEnv_WithGlobalPolicyFallback(t *testing.T) {
	storageSpec := &v1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			InitMethod: v1alpha1.VolumeInitMethodDeleteFiles,
		},
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "data",
				// No per-volume init method — should use global filesystem policy
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
				},
				Aerospike: &v1alpha1.AerospikeVolumeAttachment{Path: "/opt/aerospike/data"},
			},
		},
	}

	result := buildInitVolumesEnv(storageSpec)
	expected := "deleteFiles:/opt/aerospike/data"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildInitVolumesEnv_BlockPolicyFallback(t *testing.T) {
	storageSpec := &v1alpha1.AerospikeStorageSpec{
		BlockVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			InitMethod: v1alpha1.VolumeInitMethodBlkdiscard,
		},
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			InitMethod: v1alpha1.VolumeInitMethodDeleteFiles,
		},
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name: "block-data",
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{
						Size:       "10Gi",
						VolumeMode: corev1.PersistentVolumeBlock,
					},
				},
				Aerospike: &v1alpha1.AerospikeVolumeAttachment{Path: "/dev/xvda"},
			},
		},
	}

	result := buildInitVolumesEnv(storageSpec)
	expected := "blkdiscard:/dev/xvda"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildInitVolumesEnv_MixedVolumesWithPolicy(t *testing.T) {
	storageSpec := &v1alpha1.AerospikeStorageSpec{
		FilesystemVolumePolicy: &v1alpha1.AerospikeVolumePolicy{
			InitMethod: v1alpha1.VolumeInitMethodHeaderCleanup,
		},
		Volumes: []v1alpha1.VolumeSpec{
			{
				Name:       "data",
				InitMethod: v1alpha1.VolumeInitMethodDD, // per-volume overrides
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "10Gi"},
				},
				Aerospike: &v1alpha1.AerospikeVolumeAttachment{Path: "/data"},
			},
			{
				Name: "index",
				// No per-volume init — falls back to filesystem policy
				Source: v1alpha1.VolumeSource{
					PersistentVolume: &v1alpha1.PersistentVolumeSpec{Size: "5Gi"},
				},
				Aerospike: &v1alpha1.AerospikeVolumeAttachment{Path: "/index"},
			},
			{
				Name: "logs",
				Source: v1alpha1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
				Aerospike: &v1alpha1.AerospikeVolumeAttachment{Path: "/logs"},
				// EmptyDir: no policy applies, should be skipped
			},
		},
	}

	result := buildInitVolumesEnv(storageSpec)
	expected := "dd:/data,headerCleanup:/index"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// --- PreStop lifecycle hook tests ---

func TestBuildAerospikeContainer_HasPreStopHook(t *testing.T) {
	cluster := newTestCluster()
	c := BuildAerospikeContainer(cluster, nil)

	if c.Lifecycle == nil {
		t.Fatal("expected lifecycle to be set")
	}
	if c.Lifecycle.PreStop == nil {
		t.Fatal("expected preStop hook to be set")
	}
	if c.Lifecycle.PreStop.Exec == nil {
		t.Fatal("expected preStop hook to use exec handler")
	}

	cmd := c.Lifecycle.PreStop.Exec.Command
	if len(cmd) != 2 {
		t.Fatalf("expected 2 command parts, got %d: %v", len(cmd), cmd)
	}
	if cmd[0] != "sleep" {
		t.Errorf("expected sleep, got %q", cmd[0])
	}
	expectedArg := fmt.Sprintf("%d", PreStopSleepSeconds)
	if cmd[1] != expectedArg {
		t.Errorf("sleep arg = %q, want %q", cmd[1], expectedArg)
	}
}

func TestBuildAerospikeContainer_PreStopHookWithCustomResources(t *testing.T) {
	// Verify that setting custom resources does not affect the PreStop hook
	cluster := newTestCluster()
	cluster.Spec.PodSpec = &v1alpha1.AerospikePodSpec{
		AerospikeContainerSpec: &v1alpha1.AerospikeContainerSpec{
			Resources: &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("500m"),
				},
			},
		},
	}
	c := BuildAerospikeContainer(cluster, nil)

	if c.Lifecycle == nil || c.Lifecycle.PreStop == nil {
		t.Fatal("expected preStop hook to be set even with custom resources")
	}
	if c.Lifecycle.PreStop.Exec == nil {
		t.Fatal("expected preStop exec handler")
	}
}

// --- Probe tests ---

func TestBuildAerospikeContainer_HasLivenessProbe(t *testing.T) {
	cluster := newTestCluster()
	c := BuildAerospikeContainer(cluster, nil)

	if c.LivenessProbe == nil {
		t.Fatal("expected liveness probe to be set")
	}
	if c.LivenessProbe.Exec == nil {
		t.Fatal("liveness probe should use Exec handler")
	}
	cmd := strings.Join(c.LivenessProbe.Exec.Command, " ")
	if !strings.Contains(cmd, "/usr/bin/asinfo") {
		t.Errorf("liveness probe command should contain '/usr/bin/asinfo', got %q", cmd)
	}
	if !strings.Contains(cmd, "build") {
		t.Errorf("liveness probe command should query 'build', got %q", cmd)
	}
	if !strings.Contains(cmd, fmt.Sprintf("%d", ServicePort)) {
		t.Errorf("liveness probe command should reference port %d, got %q", ServicePort, cmd)
	}
	// Liveness probe should have more generous timing than readiness
	if c.LivenessProbe.InitialDelaySeconds < c.ReadinessProbe.InitialDelaySeconds {
		t.Error("liveness probe InitialDelaySeconds should be >= readiness probe's")
	}
	if c.LivenessProbe.InitialDelaySeconds != 30 {
		t.Errorf("liveness InitialDelaySeconds = %d, want 30", c.LivenessProbe.InitialDelaySeconds)
	}
	if c.LivenessProbe.PeriodSeconds != 30 {
		t.Errorf("liveness PeriodSeconds = %d, want 30", c.LivenessProbe.PeriodSeconds)
	}
	if c.LivenessProbe.TimeoutSeconds != 5 {
		t.Errorf("liveness TimeoutSeconds = %d, want 5", c.LivenessProbe.TimeoutSeconds)
	}
	if c.LivenessProbe.FailureThreshold != 5 {
		t.Errorf("liveness FailureThreshold = %d, want 5", c.LivenessProbe.FailureThreshold)
	}
}

func TestBuildAerospikeContainer_HasReadinessProbe(t *testing.T) {
	cluster := newTestCluster()
	c := BuildAerospikeContainer(cluster, nil)

	if c.ReadinessProbe == nil {
		t.Fatal("expected readiness probe to be set")
	}
	if c.ReadinessProbe.Exec == nil {
		t.Fatal("readiness probe should use Exec handler")
	}
	cmd := strings.Join(c.ReadinessProbe.Exec.Command, " ")
	if !strings.Contains(cmd, "/usr/bin/asinfo") {
		t.Errorf("readiness probe command should contain '/usr/bin/asinfo', got %q", cmd)
	}
	if !strings.Contains(cmd, "statistics") {
		t.Errorf("readiness probe command should query 'statistics', got %q", cmd)
	}
	if !strings.Contains(cmd, "cluster_size") {
		t.Errorf("readiness probe command should check 'cluster_size', got %q", cmd)
	}
	if !strings.Contains(cmd, fmt.Sprintf("%d", ServicePort)) {
		t.Errorf("readiness probe command should reference port %d, got %q", ServicePort, cmd)
	}
	if c.ReadinessProbe.InitialDelaySeconds != 15 {
		t.Errorf("readiness InitialDelaySeconds = %d, want 15", c.ReadinessProbe.InitialDelaySeconds)
	}
	if c.ReadinessProbe.PeriodSeconds != 10 {
		t.Errorf("readiness PeriodSeconds = %d, want 10", c.ReadinessProbe.PeriodSeconds)
	}
	if c.ReadinessProbe.TimeoutSeconds != 5 {
		t.Errorf("readiness TimeoutSeconds = %d, want 5", c.ReadinessProbe.TimeoutSeconds)
	}
	if c.ReadinessProbe.FailureThreshold != 3 {
		t.Errorf("readiness FailureThreshold = %d, want 3", c.ReadinessProbe.FailureThreshold)
	}
}

func TestBuildLivenessProbe_ReturnsExecProbe(t *testing.T) {
	probe := buildLivenessProbe()
	if probe == nil {
		t.Fatal("expected non-nil probe")
	}
	if probe.Exec == nil {
		t.Fatal("expected Exec handler")
	}
	if probe.TCPSocket != nil {
		t.Error("should not use TCPSocket handler")
	}
	if len(probe.Exec.Command) != 3 {
		t.Fatalf("expected 3-element command, got %d", len(probe.Exec.Command))
	}
	if probe.Exec.Command[0] != "/bin/sh" || probe.Exec.Command[1] != "-c" {
		t.Errorf("expected [/bin/sh -c ...], got %v", probe.Exec.Command[:2])
	}
}

func TestBuildReadinessProbe_ReturnsExecProbe(t *testing.T) {
	probe := buildReadinessProbe()
	if probe == nil {
		t.Fatal("expected non-nil probe")
	}
	if probe.Exec == nil {
		t.Fatal("expected Exec handler")
	}
	if probe.TCPSocket != nil {
		t.Error("should not use TCPSocket handler")
	}
	if len(probe.Exec.Command) != 3 {
		t.Fatalf("expected 3-element command, got %d", len(probe.Exec.Command))
	}
	if probe.Exec.Command[0] != "/bin/sh" || probe.Exec.Command[1] != "-c" {
		t.Errorf("expected [/bin/sh -c ...], got %v", probe.Exec.Command[:2])
	}
}

func TestBuildAerospikeContainer_DefaultPorts(t *testing.T) {
	cluster := newTestCluster()
	c := BuildAerospikeContainer(cluster, nil)

	portMap := make(map[string]int32)
	for _, p := range c.Ports {
		portMap[p.Name] = p.ContainerPort
	}

	expectedPorts := map[string]int32{
		"service":   ServicePort,
		"fabric":    FabricPort,
		"heartbeat": HeartbeatPort,
		"info":      InfoPort,
	}

	for name, expected := range expectedPorts {
		if got, ok := portMap[name]; !ok {
			t.Errorf("missing port %q", name)
		} else if got != expected {
			t.Errorf("port %q = %d, want %d", name, got, expected)
		}
	}
}
