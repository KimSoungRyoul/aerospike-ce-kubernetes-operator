---
sidebar_position: 5
title: Monitoring
---

# Monitoring

This guide covers per-cluster Prometheus monitoring: the exporter sidecar, ServiceMonitor, PrometheusRule with built-in and custom alert rules, and metric labels.

For **operator-level** monitoring (ServiceMonitor, PrometheusRule, and Grafana dashboard for the operator itself), see the [Install — Monitoring](install.md#monitoring-optional) section.

---

## Enabling the Exporter Sidecar

Set `monitoring.enabled: true` to inject an Aerospike Prometheus exporter sidecar into every pod:

```yaml
spec:
  monitoring:
    enabled: true
```

The operator automatically:
1. Adds the exporter container to the StatefulSet pod template
2. Exposes the metrics port on each pod
3. Shares the Aerospike network namespace so the exporter can scrape `localhost:3000`

### Exporter Configuration

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` | Enable exporter sidecar |
| `exporterImage` | `aerospike/aerospike-prometheus-exporter:1.16.1` | Exporter container image |
| `port` | `9145` | Metrics port |
| `resources` | — | CPU/memory requests and limits for the exporter |
| `env` | — | Extra environment variables for the exporter container |
| `metricLabels` | — | Custom labels added to all exported metrics |

```yaml
spec:
  monitoring:
    enabled: true
    exporterImage: aerospike/aerospike-prometheus-exporter:latest
    port: 9145
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
      limits:
        cpu: 200m
        memory: 128Mi
    env:
      - name: AS_AUTH_USER
        value: "admin"
    metricLabels:
      environment: production
      team: platform
```

Metric labels are passed to the exporter via the `METRIC_LABELS` environment variable as sorted `key=value` pairs. They appear on every metric the exporter produces, which is useful for filtering in Grafana or Prometheus.

### Exporter Environment Variables

Custom environment variables can be passed to the Prometheus exporter container via the `env` field. This is useful for controlling exporter behavior such as authentication, binding configuration, or disabling specific metric categories.

```yaml
spec:
  monitoring:
    enabled: true
    exporter:
      env:
        - name: AS_PROMETHEUS_DISABLE_CLUSTER_METRICS
          value: "true"
        - name: AS_PROMETHEUS_BIND_PORT
          value: "9146"
```

Common exporter environment variables:

| Variable | Description |
|----------|-------------|
| `AS_AUTH_USER` | Aerospike username for authenticated clusters |
| `AS_AUTH_PASSWORD` | Aerospike password (prefer K8s Secret references) |
| `AS_PROMETHEUS_BIND_PORT` | Override the default metrics bind port |
| `AS_PROMETHEUS_DISABLE_CLUSTER_METRICS` | Disable cluster-level metrics collection |
| `AS_PROMETHEUS_DISABLE_NAMESPACE_METRICS` | Disable namespace-level metrics collection |

These can also be configured through the Cluster Manager UI in the Monitoring section of the Edit dialog.

---

## ServiceMonitor

When using the [Prometheus Operator](https://prometheus-operator.dev/), enable automatic `ServiceMonitor` creation so Prometheus discovers and scrapes the exporter without manual target configuration:

```yaml
spec:
  monitoring:
    enabled: true
    serviceMonitor:
      enabled: true
      interval: "30s"
      labels:
        release: prometheus    # Must match your Prometheus serviceMonitorSelector
```

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` | Create a ServiceMonitor resource |
| `interval` | `30s` | Prometheus scrape interval |
| `labels` | — | Additional labels for Prometheus discovery (must match your Prometheus Operator's `serviceMonitorSelector`) |

:::tip
To find the label selector your Prometheus instance uses, run:
```bash
kubectl get prometheus -A -o jsonpath='{.items[*].spec.serviceMonitorSelector}'
```
:::

---

## PrometheusRule

The `prometheusRule` field creates a `PrometheusRule` resource that defines alerting rules specific to this Aerospike cluster. This is separate from the operator-level PrometheusRule (configured via Helm values) -- it provides **cluster-specific** alerts.

### Built-in Alerts

When `prometheusRule.enabled` is `true` and no `customRules` are provided, the operator generates four default alert rules:

| Alert | Condition | Severity | Description |
|-------|-----------|----------|-------------|
| `AerospikeNodeDown` | Pod target is down for 5m | critical | An Aerospike node is unreachable |
| `AerospikeStopWrites` | `stop_writes` metric is 1 for 1m | critical | Namespace has stopped accepting writes |
| `AerospikeHighDiskUsage` | Disk usage > 80% for 10m | warning | Namespace disk usage is approaching the high-water mark |
| `AerospikeHighMemoryUsage` | Memory usage > 80% for 10m | warning | Namespace memory usage is approaching the high-water mark |

```yaml
spec:
  monitoring:
    enabled: true
    serviceMonitor:
      enabled: true
    prometheusRule:
      enabled: true
      labels:
        release: prometheus    # Must match your Prometheus ruleSelector
```

### Custom Rules

To replace the built-in alerts entirely with your own rules, use the `customRules` field. When `customRules` is set, the four default alerts are **not** generated.

Each entry in `customRules` must be a complete [Prometheus rule group](https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/#rule_group) object containing `name` and `rules` fields:

```yaml
spec:
  monitoring:
    enabled: true
    serviceMonitor:
      enabled: true
    prometheusRule:
      enabled: true
      labels:
        release: prometheus
      customRules:
        - name: aerospike-custom-alerts
          rules:
            - alert: AerospikeHighClientConnections
              expr: aerospike_node_stats_client_connections > 10000
              for: 5m
              labels:
                severity: warning
              annotations:
                summary: "High client connections on {{ $labels.instance }}"
                description: "Aerospike node {{ $labels.instance }} has {{ $value }} client connections."
            - alert: AerospikeHighReadLatency
              expr: histogram_quantile(0.99, rate(aerospike_latencies_read_bucket[5m])) > 5
              for: 10m
              labels:
                severity: warning
              annotations:
                summary: "High read latency (p99 > 5ms) on {{ $labels.instance }}"
        - name: aerospike-recording-rules
          rules:
            - record: aerospike:namespace_memory_usage_ratio
              expr: aerospike_namespace_memory_used_bytes / aerospike_namespace_memory_size
```

:::warning
When `customRules` is provided, the built-in alerts (NodeDown, StopWrites, HighDiskUsage, HighMemoryUsage) are completely replaced. If you want to keep any of the defaults alongside your custom rules, you must include them explicitly in your `customRules` list.
:::

### PrometheusRule Fields

| Field | Default | Description |
|-------|---------|-------------|
| `enabled` | `false` | Create a PrometheusRule resource for this cluster |
| `labels` | — | Additional labels for Prometheus rule discovery |
| `customRules` | — | Custom rule groups that replace the built-in alerts; each entry must have `name` and `rules` |

---

## Complete Monitoring Example

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-production
  namespace: aerospike
spec:
  size: 4
  image: aerospike:ce-8.1.1.1

  monitoring:
    enabled: true
    exporterImage: aerospike/aerospike-prometheus-exporter:latest
    port: 9145
    resources:
      requests:
        cpu: 100m
        memory: 64Mi
      limits:
        cpu: 200m
        memory: 128Mi
    metricLabels:
      cluster: production
      region: us-east-1
    serviceMonitor:
      enabled: true
      interval: "30s"
      labels:
        release: prometheus
    prometheusRule:
      enabled: true
      labels:
        release: prometheus
      customRules:
        - name: aerospike-production-alerts
          rules:
            - alert: AerospikeNodeDown
              expr: up{job=~".*aerospike-production.*"} == 0
              for: 3m
              labels:
                severity: critical
                team: platform
              annotations:
                summary: "Aerospike node {{ $labels.instance }} is down"
            - alert: AerospikeStopWrites
              expr: aerospike_namespace_stop_writes{job=~".*aerospike-production.*"} == 1
              for: 30s
              labels:
                severity: critical
                team: platform
              annotations:
                summary: "Stop-writes on namespace {{ $labels.ns }} ({{ $labels.instance }})"
            - alert: AerospikeHighDiskUsage
              expr: aerospike_namespace_device_used_bytes / aerospike_namespace_device_total_bytes > 0.85
              for: 5m
              labels:
                severity: warning
              annotations:
                summary: "Disk usage above 85% on {{ $labels.ns }}"
            - alert: AerospikeHighMemoryUsage
              expr: aerospike_namespace_memory_used_bytes / aerospike_namespace_memory_size > 0.85
              for: 5m
              labels:
                severity: warning
              annotations:
                summary: "Memory usage above 85% on {{ $labels.ns }}"
        - name: aerospike-production-recording
          rules:
            - record: aerospike:tps:read:rate5m
              expr: rate(aerospike_namespace_client_read_success[5m])
            - record: aerospike:tps:write:rate5m
              expr: rate(aerospike_namespace_client_write_success[5m])

  aerospikeConfig:
    service:
      cluster-name: production
    namespaces:
      - name: data
        replication-factor: 2
        storage-engine:
          type: memory
          data-size: 4294967296
    network:
      service:
        port: 3000
      heartbeat:
        port: 3002
      fabric:
        port: 3001
```

Apply it:

```bash
kubectl apply -f aerospike-production.yaml
```

Verify the monitoring resources:

```bash
# Check the exporter sidecar is running
kubectl -n aerospike get pods -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{range .spec.containers[*]}{.name}{" "}{end}{"\n"}{end}'

# Check the ServiceMonitor was created
kubectl -n aerospike get servicemonitor

# Check the PrometheusRule was created
kubectl -n aerospike get prometheusrule

# Verify Prometheus is scraping the targets
kubectl port-forward -n monitoring svc/prometheus-kube-prometheus-prometheus 9090:9090
# Then open http://localhost:9090/targets and look for aerospike targets
```

---

## Monitoring in Templates

Templates can include monitoring configuration as a default for all clusters that reference them:

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeClusterTemplate
metadata:
  name: monitored-production
spec:
  monitoring:
    enabled: true
    port: 9145
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
      limits:
        cpu: 200m
        memory: 128Mi
    serviceMonitor:
      enabled: true
      interval: "30s"
    prometheusRule:
      enabled: true
```

Clusters referencing this template inherit the monitoring configuration. Per-cluster `spec.monitoring` fields override template values.
