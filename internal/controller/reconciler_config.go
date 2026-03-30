package controller

import (
	"context"
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	ackov1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/configgen"
	ackoerrors "github.com/ksr/aerospike-ce-kubernetes-operator/internal/errors"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/initcontainer"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/podutil"
	"github.com/ksr/aerospike-ce-kubernetes-operator/internal/utils"
)

func (r *AerospikeClusterReconciler) reconcileConfigMap(
	ctx context.Context,
	cluster *ackov1alpha1.AerospikeCluster,
	rack *ackov1alpha1.Rack,
	effectiveConfig *ackov1alpha1.AerospikeConfigSpec,
) error {
	log := logf.FromContext(ctx)

	cmName := utils.ConfigMapName(cluster.Name, rack.ID)

	if effectiveConfig == nil {
		// Provide a minimal default config
		effectiveConfig = &ackov1alpha1.AerospikeConfigSpec{
			Value: map[string]any{
				"service": map[string]any{
					"cluster-name": cluster.Name,
				},
				"network": map[string]any{
					"service": map[string]any{
						"address": "any",
						"port":    int(podutil.ServicePort),
					},
					"heartbeat": map[string]any{
						"mode": "mesh",
						"port": int(podutil.HeartbeatPort),
					},
					"fabric": map[string]any{
						"address": "any",
						"port":    int(podutil.FabricPort),
					},
				},
			},
		}
	}

	// Inject access-address placeholders based on network policy
	configgen.InjectAccessAddressPlaceholders(effectiveConfig.Value, cluster.Spec.AerospikeNetworkPolicy)

	// Collect all pod names across all racks for mesh seed injection
	racks := r.getRacks(cluster)
	rackSizes := make([]int32, len(racks))
	totalPods := int32(0)
	for i := range racks {
		rackSizes[i] = r.getRackSize(cluster, racks, i)
		totalPods += rackSizes[i]
	}
	allPodNames := make([]string, 0, totalPods)
	for i, rk := range racks {
		stsName := utils.StatefulSetName(cluster.Name, rk.ID)
		for j := range rackSizes[i] {
			allPodNames = append(allPodNames, fmt.Sprintf("%s-%d", stsName, j))
		}
	}

	// Determine heartbeat port
	heartbeatPort := int(podutil.HeartbeatPort)
	if netCfg, ok := effectiveConfig.Value["network"].(map[string]any); ok {
		if hbCfg, ok := netCfg["heartbeat"].(map[string]any); ok {
			if port, ok := hbCfg["port"]; ok {
				heartbeatPort = utils.IntFromAny(port, heartbeatPort)
			}
		}
	}

	serviceName := utils.HeadlessServiceName(cluster.Name)

	// Generate aerospike.conf with mesh seeds injected
	confText, err := configgen.GenerateConfForPod(
		effectiveConfig.Value,
		serviceName,
		cluster.Namespace,
		allPodNames,
		heartbeatPort,
	)
	if err != nil {
		return ackoerrors.NewValidationf("generating aerospike.conf: %v", err)
	}

	// Build ConfigMap data
	data := initcontainer.GetConfigMapData(confText)

	labels := utils.LabelsForRack(cluster.Name, rack.ID)

	// Check if ConfigMap exists
	existing := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: cmName, Namespace: cluster.Namespace}, existing)

	if errors.IsNotFound(err) {
		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cmName,
				Namespace: cluster.Namespace,
				Labels:    labels,
			},
			Data: data,
		}
		if err := r.setOwnerRef(cluster, cm); err != nil {
			return err
		}
		log.Info("Creating ConfigMap", "name", cmName)
		if err := r.Create(ctx, cm); err != nil {
			return fmt.Errorf("creating ConfigMap %s: %w", cmName, err)
		}
		r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventConfigMapCreated,
			"ConfigMap %s created for rack %d", cmName, rack.ID)
		return nil
	} else if err != nil {
		return fmt.Errorf("getting ConfigMap %s: %w", cmName, err)
	}

	// Update only if data or labels changed
	if maps.Equal(existing.Data, data) && maps.Equal(existing.Labels, labels) {
		return nil
	}
	existing.Data = data
	existing.Labels = labels
	log.Info("Updating ConfigMap", "name", cmName)
	if err := r.Update(ctx, existing); err != nil {
		return fmt.Errorf("updating ConfigMap %s: %w", cmName, err)
	}
	r.Recorder.Eventf(cluster, corev1.EventTypeNormal, EventConfigMapUpdated,
		"ConfigMap %s updated with new configuration", cmName)
	return nil
}

// getEffectiveConfig returns the merged config for a rack.
func (r *AerospikeClusterReconciler) getEffectiveConfig(
	cluster *ackov1alpha1.AerospikeCluster,
	rack *ackov1alpha1.Rack,
) *ackov1alpha1.AerospikeConfigSpec {
	if cluster.Spec.AerospikeConfig == nil {
		if rack.AerospikeConfig != nil {
			return rack.AerospikeConfig
		}
		return nil
	}

	if rack.AerospikeConfig == nil {
		return cluster.Spec.AerospikeConfig
	}

	merged := utils.DeepMerge(
		cluster.Spec.AerospikeConfig.Value,
		rack.AerospikeConfig.Value,
	)
	return &ackov1alpha1.AerospikeConfigSpec{Value: merged}
}
