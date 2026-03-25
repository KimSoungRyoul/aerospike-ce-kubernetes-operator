---
sidebar_position: 4.5
title: 모니터링
---

# 모니터링

이 가이드에서는 클러스터별 Prometheus 모니터링 설정을 다룹니다: exporter 사이드카, ServiceMonitor, 기본 및 커스텀 알림 규칙이 포함된 PrometheusRule, 메트릭 라벨.

**오퍼레이터 수준**의 모니터링(오퍼레이터 자체를 위한 ServiceMonitor, PrometheusRule, Grafana 대시보드)은 [설치 — 모니터링](install.md#모니터링-선택사항) 섹션을 참조하세요.

---

## Exporter 사이드카 활성화

`monitoring.enabled: true`를 설정하면 모든 Pod에 Aerospike Prometheus exporter 사이드카가 주입됩니다:

```yaml
spec:
  monitoring:
    enabled: true
```

오퍼레이터는 자동으로:
1. StatefulSet Pod 템플릿에 exporter 컨테이너를 추가합니다
2. 각 Pod에서 메트릭 포트를 노출합니다
3. Aerospike 네트워크 네임스페이스를 공유하여 exporter가 `localhost:3000`을 스크레이핑할 수 있도록 합니다

### Exporter 설정

| 필드 | 기본값 | 설명 |
|------|--------|------|
| `enabled` | `false` | Exporter 사이드카 활성화 |
| `exporterImage` | `aerospike/aerospike-prometheus-exporter:1.16.1` | Exporter 컨테이너 이미지 |
| `port` | `9145` | 메트릭 포트 |
| `resources` | — | Exporter의 CPU/메모리 requests와 limits |
| `env` | — | Exporter 컨테이너의 추가 환경 변수 |
| `metricLabels` | — | 모든 내보내기 메트릭에 추가되는 커스텀 라벨 |

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

메트릭 라벨은 `METRIC_LABELS` 환경 변수를 통해 정렬된 `key=value` 쌍으로 exporter에 전달됩니다. 이 라벨은 exporter가 생성하는 모든 메트릭에 나타나며, Grafana나 Prometheus에서의 필터링에 유용합니다.

### Exporter 환경 변수

`env` 필드를 통해 Prometheus exporter 컨테이너에 커스텀 환경 변수를 전달할 수 있습니다. 이는 인증, 바인딩 설정, 특정 메트릭 카테고리 비활성화 등 exporter 동작을 제어하는 데 유용합니다.

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

주요 exporter 환경 변수:

| 변수 | 설명 |
|------|------|
| `AS_AUTH_USER` | 인증이 활성화된 클러스터의 Aerospike 사용자 이름 |
| `AS_AUTH_PASSWORD` | Aerospike 비밀번호 (K8s Secret 참조 권장) |
| `AS_PROMETHEUS_BIND_PORT` | 기본 메트릭 바인딩 포트 오버라이드 |
| `AS_PROMETHEUS_DISABLE_CLUSTER_METRICS` | 클러스터 수준 메트릭 수집 비활성화 |
| `AS_PROMETHEUS_DISABLE_NAMESPACE_METRICS` | 네임스페이스 수준 메트릭 수집 비활성화 |

이 설정은 클러스터 매니저 UI의 Edit 다이얼로그 Monitoring 섹션에서도 구성할 수 있습니다.

---

## ServiceMonitor

[Prometheus Operator](https://prometheus-operator.dev/)를 사용할 때, 자동 `ServiceMonitor` 생성을 활성화하면 수동 타겟 설정 없이 Prometheus가 exporter를 발견하고 스크레이핑합니다:

```yaml
spec:
  monitoring:
    enabled: true
    serviceMonitor:
      enabled: true
      interval: "30s"
      labels:
        release: prometheus    # Prometheus의 serviceMonitorSelector와 일치해야 합니다
```

| 필드 | 기본값 | 설명 |
|------|--------|------|
| `enabled` | `false` | ServiceMonitor 리소스 생성 |
| `interval` | `30s` | Prometheus 스크레이핑 주기 |
| `labels` | — | Prometheus 디스커버리용 추가 라벨 |

---

## PrometheusRule

`prometheusRule` 필드는 이 Aerospike 클러스터에 특화된 알림 규칙을 정의하는 `PrometheusRule` 리소스를 생성합니다. 이것은 오퍼레이터 수준의 PrometheusRule(Helm values로 설정)과는 별개이며, **클러스터별** 알림을 제공합니다.

### 기본 알림

`prometheusRule.enabled`가 `true`이고 `customRules`가 제공되지 않으면, 오퍼레이터는 네 가지 기본 알림 규칙을 생성합니다:

| 알림 | 조건 | 심각도 | 설명 |
|------|------|--------|------|
| `AerospikeNodeDown` | 5분간 Pod 타겟 다운 | critical | Aerospike 노드에 도달할 수 없음 |
| `AerospikeStopWrites` | `stop_writes` 메트릭이 1분간 1 | critical | 네임스페이스가 쓰기를 중지함 |
| `AerospikeHighDiskUsage` | 10분간 디스크 사용률 > 80% | warning | 네임스페이스 디스크 사용량이 HWM에 근접 |
| `AerospikeHighMemoryUsage` | 10분간 메모리 사용률 > 80% | warning | 네임스페이스 메모리 사용량이 HWM에 근접 |

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
```

### 커스텀 규칙

기본 알림을 사용자 정의 규칙으로 완전히 대체하려면 `customRules` 필드를 사용합니다. `customRules`가 설정되면 네 가지 기본 알림은 **생성되지 않습니다**.

`customRules`의 각 항목은 `name`과 `rules` 필드를 포함하는 완전한 [Prometheus 규칙 그룹](https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/#rule_group) 객체여야 합니다:

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
                summary: "{{ $labels.instance }}에서 높은 클라이언트 연결 수"
            - alert: AerospikeHighReadLatency
              expr: histogram_quantile(0.99, rate(aerospike_latencies_read_bucket[5m])) > 5
              for: 10m
              labels:
                severity: warning
              annotations:
                summary: "{{ $labels.instance }}에서 높은 읽기 지연 (p99 > 5ms)"
        - name: aerospike-recording-rules
          rules:
            - record: aerospike:namespace_memory_usage_ratio
              expr: aerospike_namespace_memory_used_bytes / aerospike_namespace_memory_size
```

:::warning
`customRules`가 제공되면 기본 알림(NodeDown, StopWrites, HighDiskUsage, HighMemoryUsage)이 완전히 대체됩니다. 기본값 중 일부를 커스텀 규칙과 함께 유지하려면 `customRules` 목록에 명시적으로 포함해야 합니다.
:::

### PrometheusRule 필드

| 필드 | 기본값 | 설명 |
|------|--------|------|
| `enabled` | `false` | 이 클러스터용 PrometheusRule 리소스 생성 |
| `labels` | — | Prometheus 규칙 디스커버리용 추가 라벨 |
| `customRules` | — | 기본 알림을 대체하는 커스텀 규칙 그룹; 각 항목에 `name`과 `rules` 필수 |

---

## 전체 모니터링 예시

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

적용:

```bash
kubectl apply -f aerospike-production.yaml
```

모니터링 리소스 확인:

```bash
# Exporter 사이드카가 실행 중인지 확인
kubectl -n aerospike get pods -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{range .spec.containers[*]}{.name}{" "}{end}{"\n"}{end}'

# ServiceMonitor 생성 확인
kubectl -n aerospike get servicemonitor

# PrometheusRule 생성 확인
kubectl -n aerospike get prometheusrule

# Prometheus가 타겟을 스크레이핑하고 있는지 확인
kubectl port-forward -n monitoring svc/prometheus-kube-prometheus-prometheus 9090:9090
# http://localhost:9090/targets에서 aerospike 타겟을 확인하세요
```

---

## 템플릿에서의 모니터링

템플릿에 모니터링 설정을 포함하여 해당 템플릿을 참조하는 모든 클러스터의 기본값으로 사용할 수 있습니다:

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
    serviceMonitor:
      enabled: true
      interval: "30s"
    prometheusRule:
      enabled: true
```

이 템플릿을 참조하는 클러스터는 모니터링 설정을 상속받습니다. 클러스터별 `spec.monitoring` 필드가 템플릿 값을 오버라이드합니다.
