---
sidebar_position: 2
title: 네트워킹
---

# 네트워킹

이 가이드에서는 오퍼레이터의 네트워킹 기능을 다룹니다: Aerospike 네트워크 접근 정책, 자동 Kubernetes NetworkPolicy 생성, CNI 대역폭 쉐이핑, LoadBalancer를 통한 외부 시드 디스커버리, 커스텀 서비스 메타데이터.

## AerospikeNetworkPolicy

`spec.aerospikeNetworkPolicy`는 Aerospike가 클라이언트 및 피어 노드에 자신의 주소를 어떻게 알리는지를 제어합니다. 클라이언트가 Kubernetes 클러스터 외부에서 접속하는 하이브리드 환경에서 중요합니다.

### 접근 타입

각 필드는 다음 네 가지 값 중 하나를 허용합니다:

| 값 | 설명 |
|---|---|
| `pod` | Pod IP를 사용합니다 (기본값). 클러스터 내부 클라이언트에 적합합니다. |
| `hostInternal` | Kubernetes 노드의 내부 IP를 사용합니다. 동일 사설 네트워크의 클라이언트용입니다. |
| `hostExternal` | Kubernetes 노드의 외부 IP를 사용합니다. 클라우드 VPC 외부의 클라이언트용입니다. |
| `configuredIP` | 파드 어노테이션에서 커스텀 IP를 사용합니다. 고급 멀티 네트워크 구성(예: Multus)용입니다. |

### 필드

| 필드 | 기본값 | 설명 |
|---|---|---|
| `accessType` | `pod` | 클라이언트가 Aerospike 서비스 포트(3000)에 접근하는 방식 |
| `alternateAccessType` | `pod` | 대체 네트워크의 클라이언트가 서비스 포트에 접근하는 방식 |
| `fabricType` | `pod` | Aerospike 노드 간 통신(fabric/heartbeat) 방식 |

### 예제: 클러스터 내부 전용 (기본값)

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: my-cluster
  namespace: aerospike
spec:
  size: 3
  image: aerospike:ce-8.1.1.1
  aerospikeNetworkPolicy:
    accessType: pod
    fabricType: pod
```

### 예제: 호스트 네트워크를 통한 외부 클라이언트 접근

클라이언트가 클러스터 외부에서 접속할 때, 노드의 외부 IP를 알립니다:

```yaml
spec:
  aerospikeNetworkPolicy:
    accessType: hostExternal
    alternateAccessType: hostInternal
    fabricType: pod
```

이 구성에서:
- 외부 클라이언트는 노드의 외부 IP를 사용하여 접속합니다.
- 내부 클라이언트(대체 네트워크)는 노드의 내부 IP를 사용합니다.
- 노드 간 fabric 트래픽은 성능을 위해 파드 네트워크에 유지됩니다.

### 예제: 커스텀 네트워크 이름과 ConfiguredIP

보조 네트워크 인터페이스를 사용하는 환경(예: Multus CNI)에서는 커스텀 네트워크 이름과 함께 `configuredIP`를 사용합니다. 오퍼레이터는 지정된 네트워크 이름과 일치하는 파드의 `k8s.v1.cni.cncf.io/network-status` 어노테이션에서 IP를 읽습니다.

```yaml
spec:
  aerospikeNetworkPolicy:
    accessType: configuredIP
    customAccessNetworkNames:
      - "aerospike-sriov-network"
    alternateAccessType: configuredIP
    customAlternateAccessNetworkNames:
      - "aerospike-macvlan-network"
    fabricType: configuredIP
    customFabricNetworkNames:
      - "aerospike-sriov-network"
```

:::warning
`configuredIP`를 사용할 때는 반드시 대응하는 `custom*NetworkNames` 필드를 제공해야 합니다. 네트워크 이름이 파드의 network-status 어노테이션 항목과 일치하지 않으면 오퍼레이터가 IP를 확인하지 못합니다.
:::

## NetworkPolicyConfig

`spec.networkPolicyConfig`는 Kubernetes NetworkPolicy 또는 Cilium CiliumNetworkPolicy 리소스의 자동 생성을 활성화합니다. 이를 통해 Aerospike 클러스터에 필요한 트래픽만 허용하도록 네트워크 트래픽을 제한합니다.

### 생성되는 규칙

활성화하면 오퍼레이터는 다음 인그레스 규칙으로 NetworkPolicy를 생성합니다:

1. **클러스터 내부 트래픽**: Fabric(3001) 및 heartbeat(3002) 포트는 클러스터의 셀렉터 레이블과 일치하는 파드에서만 허용됩니다.
2. **클라이언트 접근**: 서비스 포트(3000)는 모든 소스에 대해 개방됩니다.
3. **메트릭** (모니터링이 활성화된 경우): 설정된 메트릭 포트가 Prometheus 스크래핑을 위해 모든 소스에 개방됩니다.

### 표준 Kubernetes NetworkPolicy

```yaml
spec:
  networkPolicyConfig:
    enabled: true
    type: kubernetes
```

### Cilium CiliumNetworkPolicy

클러스터가 Cilium을 CNI로 사용하는 경우, `CiliumNetworkPolicy`를 대신 생성할 수 있습니다:

```yaml
spec:
  networkPolicyConfig:
    enabled: true
    type: cilium
```

:::info
CiliumNetworkPolicy CRD가 클러스터에 설치되어 있지 않으면, 오퍼레이터는 메시지를 로깅하고 생성을 건너뜁니다. 오류가 발생하지 않습니다.
:::

### 비활성화

`enabled: false`로 설정하거나 필드를 완전히 제거하면 이전에 생성된 NetworkPolicy가 삭제됩니다:

```yaml
spec:
  networkPolicyConfig:
    enabled: false
```

## BandwidthConfig

`spec.bandwidthConfig`는 트래픽 쉐이핑을 위해 파드 템플릿에 CNI 대역폭 어노테이션을 주입합니다. 공유 클러스터에서 파드당 네트워크 처리량을 제한하여 noisy-neighbor 문제를 방지할 때 유용합니다.

오퍼레이터는 Cilium 대역폭 매니저 및 bandwidth CNI 플러그인 등의 CNI 플러그인이 인식하는 표준 `kubernetes.io/ingress-bandwidth` 및 `kubernetes.io/egress-bandwidth` 어노테이션을 설정합니다.

### 예제

```yaml
spec:
  bandwidthConfig:
    ingress: "1Gbps"
    egress: "500Mbps"
```

두 필드 모두 선택 사항입니다. ingress만, egress만, 또는 둘 다 설정할 수 있습니다:

```yaml
spec:
  bandwidthConfig:
    egress: "200Mbps"
```

:::note
대역폭 쉐이핑은 이 어노테이션을 지원하는 CNI 플러그인이 필요합니다. CNI가 지원하지 않으면 어노테이션은 무시됩니다.
:::

## SeedsFinderServices

`spec.seedsFinderServices`는 외부 시드 디스커버리를 위한 LoadBalancer 서비스를 생성합니다. 이를 통해 Kubernetes 클러스터 외부의 클라이언트가 LoadBalancer의 외부 IP를 통해 Aerospike 시드 노드를 검색할 수 있습니다.

### LoadBalancer 설정

| 필드 | 기본값 | 설명 |
|---|---|---|
| `annotations` | - | LoadBalancer 서비스의 커스텀 어노테이션 (예: 클라우드 프로바이더 설정) |
| `labels` | - | LoadBalancer 서비스의 커스텀 레이블 |
| `externalTrafficPolicy` | - | `Cluster` 또는 `Local`. 클라이언트 소스 IP를 보존하려면 `Local`을 사용합니다. |
| `port` | `3000` | LoadBalancer의 외부 포트 |
| `targetPort` | `3000` | 트래픽을 전달할 컨테이너 포트 |
| `loadBalancerSourceRanges` | - | 보안을 위해 특정 CIDR로 트래픽을 제한합니다. |

### 예제: 기본 LoadBalancer

```yaml
spec:
  seedsFinderServices:
    loadBalancer:
      port: 3000
```

### 예제: 제한이 있는 프로덕션 LoadBalancer

```yaml
spec:
  seedsFinderServices:
    loadBalancer:
      port: 3000
      targetPort: 3000
      externalTrafficPolicy: Local
      loadBalancerSourceRanges:
        - "10.0.0.0/8"
        - "172.16.0.0/12"
      annotations:
        service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
        service.beta.kubernetes.io/aws-load-balancer-scheme: "internal"
      labels:
        environment: production
```

## HeadlessService

`spec.headlessService`를 사용하면 오퍼레이터가 각 클러스터에 생성하는 헤드리스 서비스에 커스텀 어노테이션과 레이블을 추가할 수 있습니다. 헤드리스 서비스(`<cluster-name>-headless`)는 DNS 기반 파드 디스커버리를 가능하게 하며 항상 생성됩니다.

### 예제

```yaml
spec:
  headlessService:
    metadata:
      annotations:
        prometheus.io/scrape: "true"
        external-dns.alpha.kubernetes.io/hostname: "aerospike.example.com"
      labels:
        team: platform
```

:::note
오퍼레이터가 관리하는 레이블(예: `app.kubernetes.io/name`, `app.kubernetes.io/instance`)은 커스텀 레이블로 오버라이드할 수 없습니다. 오퍼레이터가 관리하는 키와 충돌하는 레이블 키를 지정하면 오퍼레이터의 값이 우선합니다.
:::

## PodService

`spec.podService`는 각 Aerospike 파드에 대해 개별 ClusterIP Service를 생성합니다. 안정적인 파드별 DNS 이름이 필요하거나 개별 서비스 엔드포인트가 필요한 서비스 메시와 통합할 때 유용합니다.

설정하면 오퍼레이터는 각 파드에 대해 `<pod-name>-pod`라는 이름의 Service를 생성하며, `statefulset.kubernetes.io/pod-name`을 통해 해당 특정 파드를 선택합니다.

### 예제

```yaml
spec:
  podService:
    metadata:
      annotations:
        service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
      labels:
        visibility: external
```

오퍼레이터는 스케일 다운 후 또는 스펙에서 `podService`가 제거되면 불필요한 파드 서비스를 자동으로 정리합니다.

## 전체 네트워킹 예제

다음은 여러 네트워킹 기능을 결합한 종합 예제입니다:

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: production-cluster
  namespace: aerospike
spec:
  size: 4
  image: aerospike:ce-8.1.1.1

  aerospikeNetworkPolicy:
    accessType: hostExternal
    alternateAccessType: hostInternal
    fabricType: pod

  networkPolicyConfig:
    enabled: true
    type: kubernetes

  bandwidthConfig:
    ingress: "2Gbps"
    egress: "1Gbps"

  seedsFinderServices:
    loadBalancer:
      port: 3000
      externalTrafficPolicy: Local
      loadBalancerSourceRanges:
        - "10.0.0.0/8"

  headlessService:
    metadata:
      annotations:
        external-dns.alpha.kubernetes.io/hostname: "aerospike-headless.example.com"

  podService:
    metadata:
      labels:
        mesh.istio.io/managed: "true"

  aerospikeConfig:
    service:
      proto-fd-max: 15000
    namespaces:
      - name: test
        replication-factor: 2
        storage-engine:
          type: memory
          data-size: 1073741824
    network:
      service:
        port: 3000
      fabric:
        port: 3001
      heartbeat:
        mode: mesh
        port: 3002
        interval: 150
        timeout: 10
```
