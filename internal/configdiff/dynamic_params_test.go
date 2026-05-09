package configdiff

import "testing"

func TestIsDynamic_Comprehensive(t *testing.T) {
	tests := []struct {
		name      string
		paramPath string
		want      bool
	}{
		// Service context — dynamic params
		{"service proto-fd-max", "service.proto-fd-max", true},
		{"service transaction-queues", "service.transaction-queues", true},
		{"service migrate-threads", "service.migrate-threads", true},
		{"service ticker-interval", "service.ticker-interval", true},

		// Network context — dynamic params
		{"network heartbeat interval", "network.heartbeat.interval", true},
		{"network heartbeat timeout", "network.heartbeat.timeout", true},
		{"network fabric send-threads", "network.fabric.send-threads", true},

		// Namespace context — dynamic params
		{"namespace default-ttl", "namespace.default-ttl", true},
		{"namespace high-water-disk-pct", "namespace.high-water-disk-pct", true},
		{"namespace stop-writes-pct", "namespace.stop-writes-pct", true},
		{"namespace rack-id", "namespace.rack-id", true},

		// Namespace context — NOT dynamic (require synchronized cold restart;
		// applying via set-config would cause replicas to drift / data loss).
		{"namespace memory-size is NOT dynamic", "namespace.memory-size", false},
		{"namespace replication-factor is NOT dynamic", "namespace.replication-factor", false},

		// Logging context — dynamic params
		{"logging any", "logging.any", true},
		{"logging namespace", "logging.namespace", true},
		{"logging network", "logging.network", true},

		// Security context — dynamic params
		{"security report-authentication", "security.log.report-authentication", true},
		{"security report-violation", "security.log.report-violation", true},

		// Static / non-dynamic params (should return false)
		{"service cluster-name is static", "service.cluster-name", false},
		{"network service port is static", "network.service.port", false},
		{"network service address is static", "network.service.address", false},
		{"network heartbeat mode is static", "network.heartbeat.mode", false},
		{"namespace name is static", "namespace.name", false},
		{"namespace storage-engine is static", "namespace.storage-engine", false},

		// Edge cases
		{"empty string", "", false},
		{"bare context without param", "service", false},
		{"bare param without context", "proto-fd-max", false},
		{"wrong case", "Service.proto-fd-max", false},
		{"uppercase", "SERVICE.PROTO-FD-MAX", false},
		{"trailing dot", "service.proto-fd-max.", false},
		{"leading dot", ".service.proto-fd-max", false},
		{"non-existent namespace param", "namespace.xdr-enabled", false},
		{"non-existent top-level", "xdr.enabled", false},
		{"spaces", " service.proto-fd-max", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsDynamic(tt.paramPath)
			if got != tt.want {
				t.Errorf("IsDynamic(%q) = %v, want %v", tt.paramPath, got, tt.want)
			}
		})
	}
}
