package configgen

import (
	"fmt"
	"strconv"
	"strings"

	v1alpha1 "github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator/api/v1alpha1"
)

// Access-address placeholders. These strings are a CONTRACT with the init
// container: aerospike-init.sh substitutes each one at pod startup, so a change
// here without a matching change there silently leaves the literal placeholder
// in aerospike.conf.
//
// Note that MY_EXTERNAL_PORT is substituted on the NodePort path only — a
// LoadBalancer serves the service port itself, so no port placeholder is emitted
// for it. See InjectAccessAddressPlaceholders.
const (
	placeholderPodIP           = "MY_POD_IP"
	placeholderNodeIP          = "MY_NODE_IP"
	placeholderExternalAddress = "MY_EXTERNAL_ADDRESS"
	placeholderExternalPort    = "MY_EXTERNAL_PORT"
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
//
// Returns a description of any user-specified value the operator overrode, so
// the caller can say so rather than discarding it silently.
func InjectAccessAddressPlaceholders(
	config map[string]any,
	policy *v1alpha1.AerospikeNetworkPolicy,
	podServiceType string,
) []string {
	if policy == nil {
		return nil
	}

	networkSection, ok := config[SectionNetwork].(map[string]any)
	if !ok {
		return nil
	}

	svcSection, ok := networkSection[SectionService].(map[string]any)
	if !ok {
		return nil
	}

	// Inject access-address based on AccessType
	if placeholder := placeholderForNetworkType(policy.AccessType); placeholder != "" {
		if _, exists := svcSection["access-address"]; !exists {
			svcSection["access-address"] = placeholder
		}
	}

	// Captured BEFORE the policy block below, so the override notices can tell a
	// value the user wrote from one alternateAccessType derived. The remedy
	// differs: the first means "your setting was ignored", the second means "your
	// alternateAccessType was ignored".
	userAddress, userSetAddress := svcSection["alternate-access-address"]

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
	//
	// The two branches MUST agree about the port, and they used not to. The init
	// container substitutes MY_EXTERNAL_PORT only on the NodePort path
	// (aerospike-init.sh); on the LoadBalancer path it resolves the address alone,
	// because a LoadBalancer serves the service port itself. So a user-specified
	// alternate-access-port — perfectly reasonable to have set while using
	// NodePort — survived a switch to LoadBalancer and made every peer advertise
	// <lb-address>:<stale-port>, a port nothing listens on. Clients then fail to
	// reach peers via --services-alternate with a connection error, which is
	// indistinguishable from the port having been dropped.
	//
	// The ADDRESS is overridden here too, and just as unconditionally — note that
	// the policy block above deliberately respects an existing value with
	// `if !exists`, and this then replaces it regardless. That is correct (a
	// per-pod LB/NodePort service is the externally reachable endpoint, so it
	// wins), but a setting quietly ignored is the same user-visible failure as one
	// quietly lost, so both halves are reported rather than only the port.
	var overrides []string
	switch podServiceType {
	case "LoadBalancer":
		if note := describeAddressOverride(svcSection, userAddress, userSetAddress,
			policy.AlternateAccessType, placeholderExternalAddress); note != "" {
			overrides = append(overrides, note)
		}
		svcSection["alternate-access-address"] = placeholderExternalAddress
		// The LoadBalancer serves the service port, so any alternate-access-port
		// is wrong by construction here. Removing it makes Aerospike fall back to
		// the service port, which is what the LoadBalancer actually listens on.
		if old, exists := svcSection["alternate-access-port"]; exists {
			delete(svcSection, "alternate-access-port")
			overrides = append(overrides, fmt.Sprintf(
				"removed network.service.alternate-access-port (%v): a LoadBalancer per-pod service "+
					"serves the service port, so an explicit alternate-access-port would advertise a "+
					"port nothing listens on", old))
		}
	case "NodePort":
		if note := describeAddressOverride(svcSection, userAddress, userSetAddress,
			policy.AlternateAccessType, placeholderNodeIP); note != "" {
			overrides = append(overrides, note)
		}
		svcSection["alternate-access-address"] = placeholderNodeIP
		// The operator creates the per-pod NodePort Service and Kubernetes
		// allocates the port, so the live Service is the only authority on what it
		// is; the init container reads it back. A user-specified value is
		// therefore overridden rather than trusted — but it is reported, not
		// discarded in silence.
		if old, exists := svcSection["alternate-access-port"]; exists && old != placeholderExternalPort {
			overrides = append(overrides, fmt.Sprintf(
				"overrode network.service.alternate-access-port (%v) with the allocated nodePort: the "+
					"operator creates the per-pod NodePort service, so the live service is the "+
					"authority on its port", old))
		}
		svcSection["alternate-access-port"] = placeholderExternalPort
	}

	networkSection[SectionService] = svcSection
	config[SectionNetwork] = networkSection
	return overrides
}

// describeAddressOverride reports what replacing alternate-access-address
// discards, or "" when nothing meaningful is lost.
//
// Nothing is lost when the value is already what we are about to write — most
// commonly alternateAccessType=hostInternal under a NodePort service, where the
// policy already produced MY_NODE_IP.
func describeAddressOverride(
	svcSection map[string]any,
	userAddress any,
	userSetAddress bool,
	alternateAccessType v1alpha1.AerospikeNetworkType,
	replacement string,
) string {
	current, exists := svcSection["alternate-access-address"]
	if !exists || current == replacement {
		return ""
	}
	if userSetAddress {
		return fmt.Sprintf(
			"overrode network.service.alternate-access-address (%v) with %s: the per-pod service is the "+
				"externally reachable endpoint, so the operator resolves the address from it",
			userAddress, replacement)
	}
	return fmt.Sprintf(
		"overrode the alternate-access-address derived from "+
			"spec.aerospikeNetworkPolicy.alternateAccessType=%s (%v) with %s: the per-pod service is the "+
			"externally reachable endpoint, so it takes precedence over the network policy",
		alternateAccessType, current, replacement)
}

// placeholderForNetworkType returns the placeholder string for the given network type.
func placeholderForNetworkType(t v1alpha1.AerospikeNetworkType) string {
	switch t {
	case v1alpha1.AerospikeNetworkTypeHostInternal, v1alpha1.AerospikeNetworkTypeHostExternal:
		return placeholderNodeIP
	case v1alpha1.AerospikeNetworkTypePod:
		return placeholderPodIP
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
