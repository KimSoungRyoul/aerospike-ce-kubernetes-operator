package controller

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	ackov1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/podutil"
	"github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/internal/utils"
)

const testImage = "aerospike:ce-8.1.1.1"

func readyPod(configHash, podSpecHash string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod-0",
			Annotations: map[string]string{
				utils.ConfigHashAnnotation:  configHash,
				utils.PodSpecHashAnnotation: podSpecHash,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: podutil.AerospikeContainerName, Image: testImage},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}
}

func stsWithAnnotations(configHash, podSpecHash string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						utils.ConfigHashAnnotation:  configHash,
						utils.PodSpecHashAnnotation: podSpecHash,
					},
				},
			},
		},
	}
}

func TestShouldWarmRestart_NilRestConfig(t *testing.T) {
	r := &AerospikeClusterReconciler{RestConfig: nil}
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{Image: testImage},
	}
	pod := readyPod("abc", "xyz")
	sts := stsWithAnnotations("abc", "xyz")

	if r.shouldWarmRestart(cluster, pod, sts) {
		t.Error("should return false when RestConfig is nil")
	}
}

func TestShouldWarmRestart_PodNotReady(t *testing.T) {
	r := &AerospikeClusterReconciler{RestConfig: &rest.Config{}}
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{Image: testImage},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "pod-0",
			Annotations: map[string]string{
				utils.PodSpecHashAnnotation: "xyz",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: podutil.AerospikeContainerName, Image: testImage},
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionFalse},
			},
		},
	}
	sts := stsWithAnnotations("abc", "xyz")

	if r.shouldWarmRestart(cluster, pod, sts) {
		t.Error("should return false when pod is not ready")
	}
}

func TestShouldWarmRestart_ImageChanged(t *testing.T) {
	r := &AerospikeClusterReconciler{RestConfig: &rest.Config{}}
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{Image: "aerospike:ce-8.2.0.0"},
	}
	pod := readyPod("abc", "xyz")
	sts := stsWithAnnotations("abc-new", "xyz")

	if r.shouldWarmRestart(cluster, pod, sts) {
		t.Error("should return false when image changed (cold restart needed)")
	}
}

func TestShouldWarmRestart_PodSpecHashChanged(t *testing.T) {
	r := &AerospikeClusterReconciler{RestConfig: &rest.Config{}}
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{Image: testImage},
	}
	pod := readyPod("abc", "xyz-old")
	sts := stsWithAnnotations("abc-new", "xyz-new")

	if r.shouldWarmRestart(cluster, pod, sts) {
		t.Error("should return false when podSpec hash changed")
	}
}

func TestShouldWarmRestart_OnlyConfigChanged(t *testing.T) {
	r := &AerospikeClusterReconciler{RestConfig: &rest.Config{}}
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{Image: testImage},
	}
	// Pod has old config hash but same podspec hash as STS
	pod := readyPod("abc-old", "xyz")
	sts := stsWithAnnotations("abc-new", "xyz")

	if !r.shouldWarmRestart(cluster, pod, sts) {
		t.Error("should return true when only config changed (same image, same podspec hash)")
	}
}

func TestShouldWarmRestart_NoPodAnnotations(t *testing.T) {
	r := &AerospikeClusterReconciler{RestConfig: &rest.Config{}}
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{Image: testImage},
	}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod-0"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{Name: podutil.AerospikeContainerName, Image: testImage},
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	}
	sts := stsWithAnnotations("abc", "xyz")

	// Pod has no annotations, so currentPodSpecHash="" and desiredPodSpecHash="xyz"
	// Since they differ and desired != "", this should return false
	if r.shouldWarmRestart(cluster, pod, sts) {
		t.Error("should return false when pod has no annotations (new pod without hashes)")
	}
}

func TestShouldWarmRestart_NoStsAnnotations(t *testing.T) {
	r := &AerospikeClusterReconciler{RestConfig: &rest.Config{}}
	cluster := &ackov1alpha1.AerospikeCluster{
		Spec: ackov1alpha1.AerospikeClusterSpec{Image: testImage},
	}
	pod := readyPod("abc", "xyz")
	sts := &appsv1.StatefulSet{
		Spec: appsv1.StatefulSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{},
			},
		},
	}

	// STS has no podspec hash annotation, so desiredPodSpecHash=""
	// The condition `desiredPodSpecHash != ""` is false, so warm restart is allowed
	if !r.shouldWarmRestart(cluster, pod, sts) {
		t.Error("should return true when STS has no podspec hash annotation")
	}
}
