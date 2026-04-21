---
sidebar_position: 2
title: AerospikeClusterTemplate API 레퍼런스
---

# AerospikeClusterTemplate API 레퍼런스

이 페이지는 `AerospikeClusterTemplate` Custom Resource Definition (CRD) 타입을 문서화합니다.

**API Group:** `acko.io`
**API Version:** `v1alpha1`
**Kind:** `AerospikeClusterTemplate`
**Short Names:** `asct`

---

## 개요

`AerospikeClusterTemplate`은 `AerospikeCluster`를 위한 재사용 가능한 설정 프로필입니다. 공유 설정(스케줄링, 스토리지, 리소스, Aerospike 설정)을 한 번 정의하고 여러 클러스터에서 `spec.templateRef`를 통해 참조할 수 있습니다.

**스냅샷 전략:** 템플릿 spec은 클러스터 생성 시 `status.templateSnapshot`에 복사됩니다. 이후 템플릿 변경 사항은 **자동으로 전파되지 않습니다**. 재동기화하려면 클러스터 객체에 `acko.io/resync-template: "true"` 어노테이션을 설정하세요.

---

## AerospikeClusterTemplate

| 필드 | 타입 | 설명 |
|---|---|---|
| `apiVersion` | string | `acko.io/v1alpha1` |
| `kind` | string | `AerospikeClusterTemplate` |
| `metadata` | [ObjectMeta](https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/object-meta/) | 표준 객체 메타데이터 |
| `spec` | [AerospikeClusterTemplateSpec](#aerospikeclustertemplatespec) | 설정 프로필 |
| `status` | [AerospikeClusterTemplateStatus](#aerospikeclustertemplatestatus) | 관측된 상태 |

---

## AerospikeClusterTemplateSpec

| 필드 | 타입 | 설명 |
|---|---|---|
| `aerospikeConfig` | [TemplateAerospikeConfig](#templateaerospikeconfig) | Aerospike 설정 기본값 |
| `scheduling` | [TemplateScheduling](#templatescheduling) | Pod 스케줄링 기본값 |
| `storage` | [TemplateStorage](#templatestorage) | 데이터 볼륨 기본값 |
| `resources` | [ResourceRequirements](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#resources) | 컨테이너 CPU/메모리 기본값 |
| `rackConfig` | [TemplateRackConfig](#templaterackconfig) | Rack 설정 기본값 |
| `image` | string | 기본 Aerospike CE 컨테이너 이미지 (예: `aerospike:ce-8.1.1.1`). 클러스터에 `spec.image`가 설정되지 않은 경우 적용됩니다. |
| `size` | integer | 기본 클러스터 크기 (1–8). 클러스터에서 `spec.size`가 `0`(미설정)인 경우 적용됩니다. |
| `monitoring` | [AerospikeMonitoringSpec](./aerospikecluster.md#aerospikemonitoringspec) | 기본 Prometheus exporter 사이드카 설정. 클러스터에 `spec.monitoring`이 설정되지 않은 경우 적용됩니다. |
| `aerospikeNetworkPolicy` | [AerospikeNetworkPolicy](./aerospikecluster.md#aerospikenetworkpolicy) | 기본 네트워크 접근 설정. 클러스터에 `spec.aerospikeNetworkPolicy`가 설정되지 않은 경우 적용됩니다. |

---

## TemplateAerospikeConfig

| 필드 | 타입 | 설명 |
|---|---|---|
| `namespaceDefaults` | object | 클러스터의 `aerospikeConfig.namespaces`에 정의된 모든 네임스페이스에 병합되는 기본 설정. 클러스터 수준 설정이 이 기본값을 오버라이드합니다. |
| `service` | object | `aerospikeConfig`의 `service` 섹션에 대한 기본값. 클러스터 수준 설정이 이 기본값을 오버라이드합니다. |
| `network` | [TemplateNetworkConfig](#templatenetworkconfig) | 네트워크 설정 기본값 |

---

## TemplateNetworkConfig

| 필드 | 타입 | 설명 |
|---|---|---|
| `heartbeat` | [TemplateHeartbeatConfig](#templateheartbeatconfig) | Heartbeat 설정 기본값 |

---

## TemplateHeartbeatConfig

| 필드 | 타입 | 설명 |
|---|---|---|
| `mode` | string | Heartbeat 모드. CE에서는 반드시 `mesh`여야 합니다. |
| `interval` | integer | Heartbeat 간격 (밀리초) |
| `timeout` | integer | Heartbeat 타임아웃 (밀리초) |

---

## TemplateScheduling

| 필드 | 타입 | 설명 |
|---|---|---|
| `podAntiAffinityLevel` | string | Pod anti-affinity 정책: `none`, `preferred`, 또는 `required`. `required`는 노드당 하나의 Aerospike pod를 강제합니다. |
| `nodeAffinity` | [NodeAffinity](https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/pod-v1/#NodeAffinity) | Pod 스케줄링을 위한 노드 어피니티 규칙 |
| `tolerations` | []Toleration | Pod 스케줄링 톨러레이션 |
| `topologySpreadConstraints` | []TopologySpreadConstraint | 토폴로지 도메인에 걸쳐 pod가 분산되는 방식 |
| `podManagementPolicy` | string | StatefulSet pod 관리 정책: `OrderedReady` 또는 `Parallel` |

### PodAntiAffinityLevel 값

| 값 | 동작 |
|---|---|
| `none` | Anti-affinity 규칙이 주입되지 않음 |
| `preferred` | Soft 규칙 (weight=100): pod를 노드 간에 분산하는 것을 선호 |
| `required` | Hard 규칙: 노드당 정확히 하나의 Aerospike pod |

---

## TemplateStorage

| 필드 | 타입 | 설명 |
|---|---|---|
| `storageClassName` | string | 데이터 PVC를 위한 Kubernetes StorageClass |
| `volumeMode` | string | `Filesystem` (기본값) 또는 `Block` |
| `accessModes` | []string | PVC 접근 모드 (기본값: `ReadWriteOnce`) |
| `resources` | [VolumeResourceRequirements](https://kubernetes.io/docs/reference/kubernetes-api/config-and-storage-resources/persistent-volume-claim-v1/#VolumeResourceRequirements) | 스토리지 크기 요청 |
| `localPVRequired` | boolean | `true`인 경우, StorageClass가 `WaitForFirstConsumer` 바인딩 모드(로컬 PV)를 사용하는지 확인합니다 |

---

## TemplateRackConfig

| 필드 | 타입 | 설명 |
|---|---|---|
| `maxRacksPerNode` | integer | Kubernetes 노드당 최대 rack 수. `1`로 설정된 경우, `podAntiAffinityLevel`이 `required`가 아니면 경고가 발생합니다. |

---

## AerospikeClusterTemplateStatus

| 필드 | 타입 | 설명 |
|---|---|---|
| `usedBy` | []string | 이 템플릿을 참조하는 `AerospikeCluster` 이름 목록 |

---

## 클러스터에서 템플릿 사용

`AerospikeCluster`의 `spec.templateRef`를 통해 템플릿을 참조합니다:

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: my-cluster
spec:
  size: 3
  image: aerospike:ce-8.1.1.1
  templateRef:
    name: prod          # "prod"라는 이름의 AerospikeClusterTemplate을 참조
  overrides:            # 선택 사항: 템플릿의 특정 필드를 오버라이드
    resources:
      requests:
        cpu: "1"
        memory: "2Gi"
      limits:
        cpu: "2"
        memory: "2Gi"
```

### 템플릿 업데이트 후 재동기화

템플릿 변경 사항은 클러스터에 자동으로 적용되지 않습니다. 재동기화하려면:

```bash
kubectl annotate aerospikecluster my-cluster acko.io/resync-template=true
```

오퍼레이터가 템플릿을 다시 가져오고, `status.templateSnapshot`을 업데이트하고, `TemplateApplied` 이벤트를 발행한 후 어노테이션을 제거합니다.

---

## 검증 규칙

| 규칙 | 설명 |
|---|---|
| V-T01 | `scheduling.podAntiAffinityLevel`은 `none`, `preferred`, 또는 `required`여야 합니다 |
| V-T02 | `rackConfig.maxRacksPerNode`은 0 이상이어야 합니다 |
| V-T03 | `storageClassName` 없이 `storage.localPVRequired=true`를 설정하면 경고가 발생합니다 |
| V-T04 | Guaranteed QoS를 위해 리소스 requests는 limits와 같아야 합니다 (경고) |
| V-T05 | `scheduling.podManagementPolicy`는 `OrderedReady` 또는 `Parallel`이어야 합니다 |
| V-T06 | `image`는 Community Edition 이미지임을 확인하기 위해 `ce-`를 포함해야 합니다 (경고) |
| V-T07 | `size`가 지정된 경우 1에서 8 사이여야 합니다 (CE 클러스터 제한) |
| V-T08 | `monitoring.port`가 지정된 경우 1에서 65535 사이여야 합니다 |
