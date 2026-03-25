---
sidebar_position: 5
title: 고급 설정
---

# 고급 설정

이 가이드에서는 Aerospike 클러스터 동작의 세부 조정, 파드 스케줄링, 랙 수준 오버라이드, 검증 정책 등 오퍼레이터의 고급 기능을 다룹니다.

---

## EnableRackIDOverride

기본적으로 오퍼레이터는 StatefulSet 순서 번호와 랙 정의에 따라 파드에 랙 ID를 할당합니다. `enableRackIDOverride`를 활성화하면 파드 어노테이션을 통한 동적 랙 ID 할당이 가능해져, 파드가 속할 랙을 수동으로 제어할 수 있습니다.

이 기능은 스케일 다운 후 다시 스케일 업하지 않고 파드를 랙 간에 마이그레이션해야 하거나, 랙 배치를 관리하는 외부 오케스트레이션 도구와 통합할 때 유용합니다.

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-cluster
spec:
  size: 4
  image: aerospike:ce-8.1.1.1
  enableRackIDOverride: true
  rackConfig:
    racks:
      - id: 1
        zone: us-east-1a
      - id: 2
        zone: us-east-1b
  aerospikeConfig:
    service:
      cluster-name: my-cluster
    namespaces:
      - name: test
        memory-size: 1073741824
        replication-factor: 2
        storage-engine:
          type: memory
    network:
      service:
        port: 3000
      heartbeat:
        port: 3002
      fabric:
        port: 3001
```

:::caution
`enableRackIDOverride`는 수동 랙 할당이 필요한 특정 상황에서만 사용하세요. 대부분의 경우 오퍼레이터의 자동 랙 분배로 충분하며 더 안전합니다.
:::

---

## ValidationPolicy

`validationPolicy` 필드는 오퍼레이터가 수행하는 검증 확인을 제어합니다. 현재 `skipWorkDirValidate` 옵션 하나를 지원합니다.

### skipWorkDirValidate

기본적으로 오퍼레이터는 Aerospike 작업 디렉토리(`/opt/aerospike`)가 영구 스토리지로 백업되어 있는지 검증합니다. 이를 통해 임시 볼륨으로 인한 데이터 손실을 방지합니다. `skipWorkDirValidate: true`로 설정하면 이 검사를 비활성화합니다.

**사용 시기:**

- 영속성이 필요 없는 개발 또는 테스트 클러스터
- 작업 디렉토리에 `emptyDir` 또는 호스트 경로 볼륨을 사용하는 클러스터
- 임시 벤치마킹 환경

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-dev
spec:
  size: 1
  image: aerospike:ce-8.1.1.1
  validationPolicy:
    skipWorkDirValidate: true
  aerospikeConfig:
    service:
      cluster-name: dev-cluster
    namespaces:
      - name: test
        memory-size: 1073741824
        replication-factor: 1
        storage-engine:
          type: memory
    network:
      service:
        port: 3000
      heartbeat:
        port: 3002
      fabric:
        port: 3001
```

---

## 파드 스케줄링

`spec.podSpec` 필드는 Kubernetes 스케줄링 프리미티브에 대한 완전한 제어를 제공합니다. 모든 표준 Kubernetes 스케줄링 필드를 지원합니다.

### nodeSelector

특정 레이블이 있는 노드에 Aerospike 파드를 제한합니다.

```yaml
spec:
  podSpec:
    nodeSelector:
      disktype: ssd
      kubernetes.io/arch: amd64
```

### Tolerations

Taint가 설정된 노드에 Aerospike 파드를 스케줄링할 수 있도록 허용합니다.

```yaml
spec:
  podSpec:
    tolerations:
      - key: "dedicated"
        operator: "Equal"
        value: "aerospike"
        effect: "NoSchedule"
      - key: "node.kubernetes.io/disk-pressure"
        operator: "Exists"
        effect: "NoSchedule"
```

### Affinity

정밀한 스케줄링 제어를 위해 파드 어피니티 및 안티어피니티 규칙을 정의합니다.

```yaml
spec:
  podSpec:
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchExpressions:
                  - key: app
                    operator: In
                    values:
                      - aerospike
              topologyKey: kubernetes.io/hostname
      nodeAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          nodeSelectorTerms:
            - matchExpressions:
                - key: topology.kubernetes.io/zone
                  operator: In
                  values:
                    - us-east-1a
                    - us-east-1b
```

:::tip
`podSpec.multiPodPerHost`가 `false`이거나 (`hostNetwork: true`인 경우 `nil`일 때) 오퍼레이터는 노드당 하나의 Aerospike 파드만 배치되도록 `RequiredDuringSchedulingIgnoredDuringExecution` 파드 안티어피니티 규칙을 자동으로 주입합니다. 이를 수동으로 설정할 필요는 없습니다.
:::

### topologySpreadConstraints

장애 도메인(존, 노드 등)에 파드를 균등하게 분배합니다.

```yaml
spec:
  podSpec:
    topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: topology.kubernetes.io/zone
        whenUnsatisfiable: DoNotSchedule
        labelSelector:
          matchLabels:
            app: aerospike
      - maxSkew: 1
        topologyKey: kubernetes.io/hostname
        whenUnsatisfiable: ScheduleAnyway
        labelSelector:
          matchLabels:
            app: aerospike
```

### podManagementPolicy

스케일링 시 파드 생성 방식을 제어합니다. 유효한 값:

| 값 | 동작 |
|-------|----------|
| `OrderedReady` | 파드가 순차적으로 생성되며, 다음 파드가 시작되기 전에 각 파드가 준비 상태여야 합니다 (기본값) |
| `Parallel` | 모든 파드가 동시에 생성되며, 초기 배포를 빠르게 할 때 유용합니다 |

```yaml
spec:
  podSpec:
    podManagementPolicy: Parallel
```

### priorityClassName

Aerospike 파드에 [PriorityClass](https://kubernetes.io/docs/concepts/scheduling-eviction/pod-priority-preemption/)를 설정합니다. PriorityClass는 클러스터에 미리 존재해야 합니다.

```yaml
spec:
  podSpec:
    priorityClassName: high-priority
```

### 전체 파드 스케줄링 예제

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-production
spec:
  size: 6
  image: aerospike:ce-8.1.1.1
  podSpec:
    podManagementPolicy: OrderedReady
    nodeSelector:
      disktype: ssd
    tolerations:
      - key: "dedicated"
        operator: "Equal"
        value: "aerospike"
        effect: "NoSchedule"
    affinity:
      podAntiAffinity:
        requiredDuringSchedulingIgnoredDuringExecution:
          - labelSelector:
              matchExpressions:
                - key: app
                  operator: In
                  values:
                    - aerospike
            topologyKey: kubernetes.io/hostname
    topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: topology.kubernetes.io/zone
        whenUnsatisfiable: DoNotSchedule
        labelSelector:
          matchLabels:
            app: aerospike
    terminationGracePeriodSeconds: 600
    priorityClassName: high-priority
  aerospikeConfig:
    service:
      cluster-name: production
    namespaces:
      - name: data
        memory-size: 4294967296
        replication-factor: 2
        storage-engine:
          type: memory
    network:
      service:
        port: 3000
      heartbeat:
        port: 3002
      fabric:
        port: 3001
```

---

## 파드 커스터마이징

`spec.podSpec` 필드는 스케줄링 외에도 여러 Kubernetes 파드 수준 기능에 대한 접근을 제공합니다. 이 섹션에서는 사이드카 컨테이너, init 컨테이너, 보안 컨텍스트, 서비스 어카운트, 이미지 풀 시크릿을 다룹니다.

### 커스텀 사이드카

모든 Aerospike 파드에 사이드카 컨테이너를 추가합니다. 메인 Aerospike 서버 컨테이너와 함께 실행되며 파드의 네트워크 네임스페이스와 볼륨을 공유합니다.

```yaml
spec:
  podSpec:
    sidecars:
      - name: log-forwarder
        image: fluent/fluent-bit:latest
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 100m
            memory: 128Mi
        volumeMounts:
          - name: aerospike-logs
            mountPath: /var/log/aerospike
            readOnly: true
  storage:
    volumes:
      - name: aerospike-logs
        source:
          emptyDir: {}
        aerospike:
          path: /var/log/aerospike
        sidecars:
          - containerName: log-forwarder
            path: /var/log/aerospike
            readOnly: true
```

:::tip
모니터링 exporter 사이드카(`spec.monitoring.enabled: true`)를 사용할 때, 오퍼레이터가 자동으로 추가합니다. `sidecars` 목록에 포함할 필요가 없습니다.
:::

### Init 컨테이너

Aerospike 서버가 시작되기 전에 실행되는 init 컨테이너를 추가합니다. 오퍼레이터의 내장 init 컨테이너(볼륨 초기화 처리) 이후에 실행됩니다.

**사용 사례:** 데이터 사전 채우기, 파일 권한 설정, 외부 소스에서 설정 다운로드, 의존성 대기.

```yaml
spec:
  podSpec:
    initContainers:
      - name: set-permissions
        image: busybox:1.36
        command: ["sh", "-c", "chown -R 1000:1000 /opt/aerospike/data"]
        volumeMounts:
          - name: data-vol
            mountPath: /opt/aerospike/data
  storage:
    volumes:
      - name: data-vol
        source:
          persistentVolume:
            storageClass: standard
            size: 50Gi
        aerospike:
          path: /opt/aerospike/data
        initContainers:
          - containerName: set-permissions
            path: /opt/aerospike/data
```

### 보안 컨텍스트

조직의 보안 정책을 충족하기 위해 파드 수준 및 컨테이너 수준 보안 속성을 설정합니다.

**파드 수준 보안 컨텍스트**는 파드의 모든 컨테이너에 적용됩니다:

```yaml
spec:
  podSpec:
    securityContext:
      runAsUser: 1000
      runAsGroup: 1000
      fsGroup: 1000
      runAsNonRoot: true
```

**컨테이너 수준 보안 컨텍스트**는 Aerospike 서버 컨테이너에만 적용됩니다:

```yaml
spec:
  podSpec:
    aerospikeContainer:
      securityContext:
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: false
        capabilities:
          drop:
            - ALL
```

### 서비스 어카운트

Aerospike 파드에 커스텀 ServiceAccount를 지정합니다. 파드가 클라우드 프로바이더 API에 접근해야 할 때(예: IAM 역할을 통한 백업/복원) 또는 제한된 RBAC가 있는 네임스페이스에서 실행할 때 유용합니다.

```yaml
spec:
  podSpec:
    serviceAccountName: aerospike-sa
```

### 이미지 풀 시크릿

컨테이너 레지스트리 자격 증명이 포함된 Kubernetes Secret을 참조합니다. 프라이빗 레지스트리에서 이미지를 가져올 때 필요합니다.

```yaml
spec:
  podSpec:
    imagePullSecrets:
      - name: my-registry-secret
```

먼저 시크릿을 생성합니다:

```bash
kubectl -n aerospike create secret docker-registry my-registry-secret \
  --docker-server=registry.example.com \
  --docker-username=myuser \
  --docker-password=mypassword
```

---

## Secret 및 ConfigMap 볼륨

PVC, emptyDir, hostPath 볼륨 외에도 Kubernetes Secret과 ConfigMap을 Aerospike 파드에 직접 마운트할 수 있습니다. TLS 인증서, 커스텀 설정 파일, 자격 증명 주입에 유용합니다.

### Secret 볼륨

Kubernetes Secret을 Aerospike 컨테이너의 파일로 마운트합니다:

```yaml
spec:
  storage:
    volumes:
      - name: aerospike-creds
        source:
          secret:
            secretName: aerospike-credentials
            items:
              - key: tls.crt
                path: tls.crt
              - key: tls.key
                path: tls.key
        aerospike:
          path: /opt/aerospike/certs
          readOnly: true
```

### ConfigMap 볼륨

추가 설정 파일을 제공하기 위해 ConfigMap을 마운트합니다:

```yaml
spec:
  storage:
    volumes:
      - name: custom-config
        source:
          configMap:
            name: aerospike-custom-config
        aerospike:
          path: /opt/aerospike/custom
          readOnly: true
```

---

## 랙 수준 오버라이드

`spec.rackConfig.racks`의 각 랙은 클러스터 수준 설정인 `aerospikeConfig`, `storage`, `podSpec`을 오버라이드할 수 있습니다. 이를 통해 장애 도메인 간에 이기종 구성이 가능합니다.

### 랙별 aerospikeConfig

랙 수준에서 Aerospike 설정을 오버라이드합니다. 랙 수준 설정은 클러스터 수준 설정과 병합되며, 랙 값이 우선합니다.

```yaml
spec:
  aerospikeConfig:
    service:
      cluster-name: my-cluster
    namespaces:
      - name: data
        memory-size: 4294967296
        replication-factor: 2
        storage-engine:
          type: memory
    network:
      service:
        port: 3000
      heartbeat:
        port: 3002
      fabric:
        port: 3001
  rackConfig:
    racks:
      - id: 1
        aerospikeConfig:
          namespaces:
            - name: data
              memory-size: 8589934592
              replication-factor: 2
              storage-engine:
                type: memory
      - id: 2
```

이 예제에서 랙 1은 `data` 네임스페이스에 8 GB 메모리를 사용하고, 랙 2는 클러스터 수준의 4 GB 설정을 상속합니다.

### 랙별 storage

특정 랙의 스토리지 설정을 오버라이드합니다. 예를 들어 다른 가용 영역에서 다른 StorageClass를 사용할 수 있습니다.

```yaml
spec:
  storage:
    volumes:
      - name: data-vol
        storageClass: gp3
        size: 50Gi
        path: /opt/aerospike/data
        volumeMode: Filesystem
  rackConfig:
    racks:
      - id: 1
        storage:
          volumes:
            - name: data-vol
              storageClass: io2
              size: 100Gi
              path: /opt/aerospike/data
              volumeMode: Filesystem
      - id: 2
```

### 랙별 podSpec (affinity, tolerations, nodeSelector)

`Rack.podSpec` 필드(`RackPodSpec`)는 세 가지 스케줄링 오버라이드를 지원합니다:

| 필드 | 타입 | 설명 |
|------|------|------|
| `affinity` | `corev1.Affinity` | 클러스터 수준 어피니티 규칙 오버라이드 |
| `tolerations` | `[]corev1.Toleration` | 클러스터 수준 톨러레이션 오버라이드 |
| `nodeSelector` | `map[string]string` | 클러스터 수준 노드 셀렉터 오버라이드 |

```yaml
spec:
  rackConfig:
    racks:
      - id: 1
        zone: us-east-1a
        podSpec:
          nodeSelector:
            topology.kubernetes.io/zone: us-east-1a
          tolerations:
            - key: "zone-a-dedicated"
              operator: "Exists"
              effect: "NoSchedule"
          affinity:
            nodeAffinity:
              requiredDuringSchedulingIgnoredDuringExecution:
                nodeSelectorTerms:
                  - matchExpressions:
                      - key: node.kubernetes.io/instance-type
                        operator: In
                        values:
                          - m5.2xlarge
                          - m5.4xlarge
      - id: 2
        zone: us-east-1b
        podSpec:
          nodeSelector:
            topology.kubernetes.io/zone: us-east-1b
```

### 랙 토폴로지 단축 필드

각 랙은 일반적인 스케줄링 시나리오를 위한 단축 필드도 제공합니다:

| 필드 | 설명 |
|------|------|
| `zone` | `topology.kubernetes.io/zone` 노드 어피니티에 해당 |
| `region` | `topology.kubernetes.io/region` 노드 어피니티에 해당 |
| `nodeName` | 랙을 특정 Kubernetes 노드로 제한 |
| `rackLabel` | `acko.io/rack=<rackLabel>` 레이블이 있는 노드에 파드를 스케줄링 |

```yaml
spec:
  rackConfig:
    racks:
      - id: 1
        zone: us-east-1a
      - id: 2
        zone: us-east-1b
      - id: 3
        region: us-west-2
        rackLabel: high-perf
```

---

## 랙 설정

`spec.rackConfig` 최상위 필드는 네임스페이스 인식, 스케일링, 롤링 업데이트에 대한 랙 전체 동작을 제어합니다.

### namespaces

랙 인식이 적용되는 Aerospike 네임스페이스를 지정합니다. 비어 있으면 모든 네임스페이스가 기본 복제 팩터를 사용합니다. CE는 최대 2개의 네임스페이스를 지원합니다.

```yaml
spec:
  rackConfig:
    namespaces:
      - data
      - cache
    racks:
      - id: 1
      - id: 2
```

### scaleDownBatchSize

스케일 다운 작업 시 랙당 동시에 제거되는 파드 수를 제어합니다. 절대 정수값 또는 백분율 문자열을 입력할 수 있습니다. 기본값은 1입니다.

```yaml
spec:
  rackConfig:
    scaleDownBatchSize: 2
    racks:
      - id: 1
      - id: 2
```

백분율 사용:

```yaml
spec:
  rackConfig:
    scaleDownBatchSize: "25%"
    racks:
      - id: 1
      - id: 2
```

### maxIgnorablePods

조정(reconciliation) 중 무시할 수 있는 Pending 또는 Failed 상태 파드의 최대 수입니다. 스케줄링 문제(예: 리소스 부족, 노드 어피니티 불일치)로 파드가 고착된 상태에서도 오퍼레이터가 정상 파드의 조정을 계속 수행하도록 할 때 유용합니다. 절대 정수값 또는 백분율 문자열을 입력할 수 있습니다.

```yaml
spec:
  rackConfig:
    maxIgnorablePods: 1
    racks:
      - id: 1
      - id: 2
```

### rollingUpdateBatchSize

롤링 재시작 시 랙당 동시에 재시작되는 파드 수를 제어합니다. 절대 정수값 또는 백분율 문자열을 입력할 수 있습니다. 기본값은 1입니다. 이 필드가 설정되면 `spec.rollingUpdateBatchSize`보다 우선합니다.

```yaml
spec:
  rackConfig:
    rollingUpdateBatchSize: 2
    racks:
      - id: 1
      - id: 2
```

:::note
`rollingUpdateBatchSize` 필드는 두 곳에 있습니다:
- `spec.rollingUpdateBatchSize` (정수만, 클러스터 전체 기본값)
- `spec.rackConfig.rollingUpdateBatchSize` (정수 또는 백분율, 랙별 오버라이드, 우선 적용)
:::

---

## 파드 메타데이터

`spec.podSpec.metadata`를 통해 Aerospike 파드에 커스텀 레이블과 어노테이션을 추가할 수 있습니다. 이는 오퍼레이터 자체 레이블 외에 관리되는 모든 파드에 적용됩니다.

다음과 같은 용도에 유용합니다:

- 서비스 메시 주입 (예: Istio 사이드카 어노테이션)
- 모니터링 레이블 셀렉터
- 비용 할당 태그
- 외부 도구 연동

```yaml
spec:
  podSpec:
    metadata:
      labels:
        team: platform
        cost-center: "12345"
        environment: production
      annotations:
        sidecar.istio.io/inject: "true"
        prometheus.io/scrape: "true"
        prometheus.io/port: "9145"
```

---

## ReadinessGateEnabled

`readinessGateEnabled`를 `true`로 설정하면, 오퍼레이터는 각 파드의 스펙에 `acko.io/aerospike-ready` 조건 타입의 커스텀 Pod Readiness Gate를 주입합니다. 오퍼레이터는 Aerospike 노드가 다음 조건을 충족한 후에만 이 게이트를 `True`로 패치합니다:

1. 클러스터 메시에 합류
2. 보류 중인 모든 데이터 마이그레이션 완료

Readiness gate가 충족되기 전까지 파드는 Service 엔드포인트에서 제외되어, 요청을 처리할 준비가 완전히 되지 않은 노드로 트래픽이 라우팅되는 것을 방지합니다.

하위 호환성을 위해 기본값은 `false`입니다.

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-with-readiness-gate
spec:
  size: 3
  image: aerospike:ce-8.1.1.1
  podSpec:
    readinessGateEnabled: true
  aerospikeConfig:
    service:
      cluster-name: my-cluster
    namespaces:
      - name: data
        memory-size: 2147483648
        replication-factor: 2
        storage-engine:
          type: memory
    network:
      service:
        port: 3000
      heartbeat:
        port: 3002
      fabric:
        port: 3001
```

개별 파드에서 readiness gate 상태를 확인할 수 있습니다:

```bash
kubectl get pod aerospike-with-readiness-gate-1-0 -o jsonpath='{.status.conditions[?(@.type=="acko.io/aerospike-ready")]}'
```

`status.pods`의 파드별 상태 필드 `readinessGateSatisfied`도 현재 게이트가 `True`인지 여부를 나타냅니다.

:::tip
프로덕션 클러스터에서는 readiness gate를 활성화하여 무중단 롤링 업데이트를 보장하세요. 롤링 재시작 중 새 파드는 클러스터에 완전히 합류하고 데이터 마이그레이션을 완료할 때까지 클라이언트 트래픽을 수신하지 않습니다.
:::
