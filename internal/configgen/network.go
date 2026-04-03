package configgen

import (
	"strconv"
	"strings"

	v1alpha1 "github.com/ksr/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

// generateNetworkSection generates the network stanza with mesh seeds injected
// for all pods in the StatefulSet.
func generateNetworkSection(
	networkConfig map[string]any,
	serviceName, namespace string,
	podNames []string,
	heartbeatPort int,
) string {
	var b strings.Builder
	b.WriteString("network {\n")

	keys := sortedKeys(networkConfig)
	for _, key := range keys {
		val := networkConfig[key]

		if key == SectionHeartbeat {
			hbMap, ok := val.(map[string]any)
			if !ok {
				hbMap = make(map[string]any)
			}
			b.WriteString(generateHeartbeatSubsection(hbMap, serviceName, namespace, podNames, heartbeatPort))
		} else if subMap, ok := val.(map[string]any); ok {
			b.WriteString("\t")
			b.WriteString(key)
			b.WriteString(" {\n")
			writeMapEntries(&b, subMap, 2)
			b.WriteString("\t}\n")
		} else {
			b.WriteString("\t")
			b.WriteString(key)
			b.WriteString(" ")
			b.WriteString(formatValue(val))
			b.WriteString("\n")
		}
	}

	b.WriteString("}\n")
	return b.String()
}

// generateHeartbeatSubsection generates the heartbeat sub-stanza with mesh seed entries.
func generateHeartbeatSubsection(
	hbConfig map[string]any,
	serviceName, namespace string,
	podNames []string,
	heartbeatPort int,
) string {
	var b strings.Builder
	b.WriteString("\theartbeat {\n")

	// Write existing heartbeat config entries (excluding mesh-seed-address-port).
	keys := sortedKeys(hbConfig)
	for _, key := range keys {
		if key == KeyMeshSeedAddressPort {
			continue
		}
		val := hbConfig[key]
		if subMap, ok := val.(map[string]any); ok {
			b.WriteString("\t\t")
			b.WriteString(key)
			b.WriteString(" {\n")
			writeMapEntries(&b, subMap, 3)
			b.WriteString("\t\t}\n")
		} else {
			b.WriteString("\t\t")
			b.WriteString(key)
			b.WriteString(" ")
			b.WriteString(formatValue(val))
			b.WriteString("\n")
		}
	}

	// Inject mesh-seed-address-port for all pods.
	dnsSuffix := "." + serviceName + "." + namespace + ".svc.cluster.local"
	portStr := strconv.Itoa(heartbeatPort)
	for _, pName := range podNames {
		b.WriteString("\t\tmesh-seed-address-port ")
		b.WriteString(pName)
		b.WriteString(dnsSuffix)
		b.WriteString(" ")
		b.WriteString(portStr)
		b.WriteString("\n")
	}

	b.WriteString("\t}\n")
	return b.String()
}

// InjectAccessAddressPlaceholders injects access-address and alternate-access-address
// placeholders into the network config based on the AerospikeNetworkPolicy.
// These placeholders (MY_POD_IP, MY_NODE_IP) are replaced by the init container
// at pod startup using Downward API environment variables.
//
// When podServiceType is LoadBalancer or NodePort, additional placeholders
// (MY_EXTERNAL_ADDRESS, MY_EXTERNAL_PORT) are injected for external access.
func InjectAccessAddressPlaceholders(config map[string]any, policy *v1alpha1.AerospikeNetworkPolicy, podServiceType string) {
	if policy == nil {
		return
	}

	networkSection, ok := config[SectionNetwork].(map[string]any)
	if !ok {
		return
	}

	svcSection, ok := networkSection[SectionService].(map[string]any)
	if !ok {
		return
	}

	// Inject access-address based on AccessType
	if placeholder := placeholderForNetworkType(policy.AccessType); placeholder != "" {
		if _, exists := svcSection["access-address"]; !exists {
			svcSection["access-address"] = placeholder
		}
	}

	// Inject alternate-access-address based on AlternateAccessType
	if placeholder := placeholderForNetworkType(policy.AlternateAccessType); placeholder != "" {
		if _, exists := svcSection["alternate-access-address"]; !exists {
			svcSection["alternate-access-address"] = placeholder
		}
	}

	// When per-pod services use LoadBalancer or NodePort, override the
	// alternate-access-address/port with external placeholders that the init
	// container resolves at startup by querying the pod's own Kubernetes Service.
	// This takes precedence over the network policy's alternateAccessType because
	// the per-pod LB/NodePort services provide the externally reachable endpoints.
	if podServiceType == "LoadBalancer" {
		svcSection["alternate-access-address"] = "MY_EXTERNAL_ADDRESS"
	}
	if podServiceType == "NodePort" {
		svcSection["alternate-access-address"] = "MY_NODE_IP"
		svcSection["alternate-access-port"] = "MY_EXTERNAL_PORT"
	}

	networkSection[SectionService] = svcSection
	config[SectionNetwork] = networkSection
}

// placeholderForNetworkType returns the placeholder string for the given network type.
func placeholderForNetworkType(t v1alpha1.AerospikeNetworkType) string {
	switch t {
	case v1alpha1.AerospikeNetworkTypeHostInternal, v1alpha1.AerospikeNetworkTypeHostExternal:
		return "MY_NODE_IP"
	case v1alpha1.AerospikeNetworkTypePod:
		return "MY_POD_IP"
	case v1alpha1.AerospikeNetworkTypeConfiguredIP:
		// configuredIP addresses are injected via pod annotations at startup,
		// not via config template placeholders. Returning "" intentionally skips
		// placeholder injection so the init container can set the address from
		// the annotation value instead.
		return ""
	default:
		return ""
	}
}
