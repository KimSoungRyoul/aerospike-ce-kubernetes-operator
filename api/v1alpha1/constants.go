package v1alpha1

// Default Aerospike network ports and container constants.
// These are used by both the webhook defaulter and podutil package
// to keep port assignments and container naming consistent across
// generated StatefulSets, Services, and ConfigMaps.
const (
	// DefaultServicePort is the default Aerospike client service port (3000).
	// Used for application connections to the cluster.
	DefaultServicePort int32 = 3000
	// DefaultFabricPort is the default Aerospike fabric port (3001).
	// Used for inter-node data replication and migration traffic.
	DefaultFabricPort int32 = 3001
	// DefaultHeartbeatPort is the default Aerospike heartbeat port (3002).
	// Used for mesh heartbeat communication between cluster nodes.
	DefaultHeartbeatPort int32 = 3002

	// AerospikeContainerName is the fixed name of the Aerospike server container
	// within each pod. Referenced by the operator when building pod specs and
	// performing rolling restarts.
	AerospikeContainerName = "aerospike-server"

	// DefaultExporterImage is the default Prometheus exporter sidecar image.
	// Used by both the webhook defaulter and template application to ensure
	// monitoring-enabled clusters always have an exporter image configured.
	DefaultExporterImage = "aerospike/aerospike-prometheus-exporter:1.16.1"
	// DefaultExporterPort is the default Prometheus exporter metrics port (9145).
	// Used by both the webhook defaulter and template application to ensure
	// monitoring-enabled clusters always have a metrics port configured.
	DefaultExporterPort int32 = 9145
)
