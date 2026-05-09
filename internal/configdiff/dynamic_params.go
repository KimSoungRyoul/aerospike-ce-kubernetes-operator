package configdiff

// dynamicParams is a registry of Aerospike CE configuration parameters that
// can be changed at runtime via the "set-config:" asinfo command without
// requiring a server restart.
//
// The key is the context-qualified parameter path used in asinfo set-config.
// For example, "service.proto-fd-max" maps to:
//
//	set-config:context=service;proto-fd-max=<value>
//
// This list is based on Aerospike CE 7.x/8.x documentation for dynamically
// configurable parameters.
var dynamicParams = map[string]bool{
	// Service context
	"service.proto-fd-max":                  true,
	"service.transaction-queues":            true,
	"service.transaction-threads-per-queue": true,
	"service.batch-index-threads":           true,
	"service.batch-max-buffers-per-queue":   true,
	"service.batch-max-unused-buffers":      true,
	"service.info-threads":                  true,
	"service.migrate-fill-delay":            true,
	"service.migrate-max-num-incoming":      true,
	"service.migrate-threads":               true,
	"service.proto-fd-idle-ms":              true,
	"service.query-threads-limit":           true,
	"service.scan-threads-limit":            true,
	"service.ticker-interval":               true,

	// Network context
	"network.heartbeat.interval":               true,
	"network.heartbeat.timeout":                true,
	"network.fabric.channel-bulk-recv-threads": true,
	"network.fabric.channel-ctrl-recv-threads": true,
	"network.fabric.channel-meta-recv-threads": true,
	"network.fabric.channel-rw-recv-threads":   true,
	"network.fabric.recv-rearm-threshold":      true,
	"network.fabric.send-threads":              true,

	// Namespace context (requires namespace name)
	// NOTE: namespace.replication-factor and namespace.memory-size (renamed to
	// data-size in CE 8.1) are NOT dynamic — they require a synchronized cold
	// restart. Applying them via set-config would cause replicas to drift and
	// risk data loss, so they are intentionally omitted; the diff engine falls
	// back to a cold restart for these params.
	"namespace.default-ttl":                 true,
	"namespace.high-water-disk-pct":         true,
	"namespace.high-water-memory-pct":       true,
	"namespace.stop-writes-pct":             true,
	"namespace.stop-writes-sys-memory-pct":  true,
	"namespace.migrate-order":               true,
	"namespace.migrate-retransmit-ms":       true,
	"namespace.migrate-sleep":               true,
	"namespace.nsup-hist-period":            true,
	"namespace.nsup-period":                 true,
	"namespace.nsup-threads":                true,
	"namespace.prefer-uniform-balance":      true,
	"namespace.rack-id":                     true,
	"namespace.read-page-cache":             true,
	"namespace.transaction-pending-limit":   true,
	"namespace.write-commit-level-override": true,

	// Logging context
	"logging.any":       true,
	"logging.misc":      true,
	"logging.alloc":     true,
	"logging.arenax":    true,
	"logging.hardware":  true,
	"logging.msg":       true,
	"logging.namespace": true,
	"logging.network":   true,
	"logging.os":        true,
	"logging.proto":     true,
	"logging.record":    true,
	"logging.socket":    true,
	"logging.xdr":       true,

	// Security context
	"security.log.report-authentication": true,
	"security.log.report-sys-admin":      true,
	"security.log.report-user-admin":     true,
	"security.log.report-violation":      true,
}

// IsDynamic returns true if the given config parameter can be changed at
// runtime without a restart.
// The paramPath should be in the form "context.param" or "context.sub.param".
func IsDynamic(paramPath string) bool {
	return dynamicParams[paramPath]
}
