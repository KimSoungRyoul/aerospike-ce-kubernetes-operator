package utils

import (
	"testing"
)

func TestStatefulSetName(t *testing.T) {
	tests := []struct {
		clusterName string
		rackID      int
		expected    string
	}{
		{"my-cluster", 0, "my-cluster-0"},
		{"my-cluster", 1, "my-cluster-1"},
		{"aero", 99, "aero-99"},
	}
	for _, tc := range tests {
		if got := StatefulSetName(tc.clusterName, tc.rackID); got != tc.expected {
			t.Errorf("StatefulSetName(%q, %d) = %q, want %q", tc.clusterName, tc.rackID, got, tc.expected)
		}
	}
}

func TestHeadlessServiceName(t *testing.T) {
	if got := HeadlessServiceName("my-cluster"); got != "my-cluster" {
		t.Errorf("HeadlessServiceName = %q, want %q", got, "my-cluster")
	}
}

func TestPodServiceName(t *testing.T) {
	if got := PodServiceName("my-cluster", 2); got != "my-cluster-2" {
		t.Errorf("PodServiceName = %q, want %q", got, "my-cluster-2")
	}
}

func TestConfigMapName(t *testing.T) {
	if got := ConfigMapName("my-cluster", 0); got != "my-cluster-0-config" {
		t.Errorf("ConfigMapName = %q, want %q", got, "my-cluster-0-config")
	}
}

func TestPDBName(t *testing.T) {
	if got := PDBName("my-cluster"); got != "my-cluster-pdb" {
		t.Errorf("PDBName = %q, want %q", got, "my-cluster-pdb")
	}
}

func TestRackPDBName(t *testing.T) {
	if got := RackPDBName("my-cluster", 2); got != "my-cluster-2-pdb" {
		t.Errorf("RackPDBName = %q, want %q", got, "my-cluster-2-pdb")
	}
	// A per-rack name must never collide with the cluster-wide one, or the
	// multi-rack path would delete the budget it just created.
	if RackPDBName("my-cluster", 0) == PDBName("my-cluster") {
		t.Error("RackPDBName collides with PDBName")
	}
}

func TestPodDNSName(t *testing.T) {
	expected := "pod-0.svc.ns.svc.cluster.local"
	if got := PodDNSName("pod-0", "svc", "ns"); got != expected {
		t.Errorf("PodDNSName = %q, want %q", got, expected)
	}
}

func TestMetricsServiceName(t *testing.T) {
	if got := MetricsServiceName("my-cluster"); got != "my-cluster-metrics" {
		t.Errorf("MetricsServiceName = %q, want %q", got, "my-cluster-metrics")
	}
}

func TestServiceMonitorName(t *testing.T) {
	if got := ServiceMonitorName("my-cluster"); got != "my-cluster-monitor" {
		t.Errorf("ServiceMonitorName = %q, want %q", got, "my-cluster-monitor")
	}
}

func TestNetworkPolicyName(t *testing.T) {
	if got := NetworkPolicyName("my-cluster"); got != "my-cluster-netpol" {
		t.Errorf("NetworkPolicyName = %q, want %q", got, "my-cluster-netpol")
	}
}

func TestPrometheusRuleName(t *testing.T) {
	if got := PrometheusRuleName("my-cluster"); got != "my-cluster-alerts" {
		t.Errorf("PrometheusRuleName = %q, want %q", got, "my-cluster-alerts")
	}
}
