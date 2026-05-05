---
sidebar_position: 5
title: AerospikeClusterTemplate
---

# 템플릿 관리

`AerospikeClusterTemplate` CRD를 사용하면 조직 전체에서 재사용 가능한 Aerospike 클러스터 설정 프로필을 정의할 수 있습니다. 이 가이드에서는 템플릿의 전체 라이프사이클(생성, 사용, 오버라이드, 동기화 동작, 모범 사례)을 다룹니다.

CRD 필드 레퍼런스는 [AerospikeClusterTemplate API Reference](../reference/api-reference/aerospikeclustertemplate.md)를 참조하세요.

---

## 템플릿 동작 원리

템플릿은 **스냅샷 모델**을 따릅니다:

1. 공유 기본값(이미지, 크기, 리소스, 스케줄링, 스토리지, 모니터링, 네트워크 정책, Aerospike 설정)을 포함하는 `AerospikeClusterTemplate`을 생성합니다.
2. `AerospikeCluster`에서 `spec.templateRef.name`으로 템플릿을 참조합니다.
3. 클러스터 생성 시 오퍼레이터가 템플릿을 해석하고 전체 스펙을 `status.templateSnapshot`에 저장합니다.
4. 이후 클러스터는 독립적으로 운영됩니다. 템플릿 변경은 **자동으로 전파되지 않습니다**.

```
┌─────────────────────┐       templateRef        ┌─────────────────────┐
│  AerospikeCluster   │ ─────────────────────►   │ AerospikeCluster    │
│  (spec.templateRef) │                          │ Template            │
└─────────────────────┘                          └─────────────────────┘
         │                                                │
         ▼                                                │
  status.templateSnapshot                                 │
  (템플릿 스펙의 고정 복사본)                               │
         │                                                │
         │   acko.io/resync-template=true                 │
         └────────────────────────────────────────────────┘
                     (수동 재동기화)
```

:::info
`AerospikeClusterTemplate`은 **클러스터 스코프** 리소스입니다. 네임스페이스에 속하지 않으며, 이름만으로 참조합니다.
:::

---

## 템플릿 생성

### Minimal 템플릿 (개발용)

개발 및 빠른 프로토타이핑을 위한 경량 템플릿:

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeClusterTemplate
metadata:
  name: minimal
spec:
  description: "개발용 템플릿 - 단일 노드, 최소 리소스"
  image: aerospike:ce-8.1.1.1
  size: 1

  resources:
    requests:
      cpu: 100m
      memory: 256Mi
    limits:
      cpu: 100m
      memory: 256Mi

  scheduling:
    podAntiAffinityLevel: none

  storage:
    storageClassName: standard
    resources:
      requests:
        storage: 1Gi

  aerospikeConfig:
    namespaceDefaults:
      replication-factor: 1
      data-size: 1073741824   # 1 GiB
```

### Soft-Rack 템플릿 (스테이징용)

소프트 anti-affinity를 사용한 랙 인식 구성으로, 스테이징 환경에 적합합니다:

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeClusterTemplate
metadata:
  name: soft-rack
spec:
  description: "스테이징 템플릿 - 소프트 anti-affinity, 3노드"
  image: aerospike:ce-8.1.1.1
  size: 3

  resources:
    requests:
      cpu: 500m
      memory: 1Gi
    limits:
      cpu: 500m
      memory: 1Gi

  scheduling:
    podAntiAffinityLevel: preferred

  storage:
    storageClassName: standard
    resources:
      requests:
        storage: 10Gi

  aerospikeConfig:
    namespaceDefaults:
      replication-factor: 2
      data-size: 2147483648   # 2 GiB
```

### Hard-Rack 템플릿 (프로덕션용)

엄격한 anti-affinity(노드당 하나의 파드), 로컬 스토리지, 모니터링을 적용합니다:

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeClusterTemplate
metadata:
  name: hard-rack
spec:
  description: "프로덕션 템플릿 - 하드 anti-affinity, 로컬 PV, 모니터링"
  image: aerospike:ce-8.1.1.1
  size: 6

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
      interval: 30s

  aerospikeNetworkPolicy:
    accessType: pod
    alternateAccessType: pod
    fabricType: pod

  scheduling:
    podAntiAffinityLevel: required
    tolerations:
      - key: "aerospike"
        operator: "Exists"
        effect: "NoSchedule"

  storage:
    storageClassName: local-path
    localPVRequired: true
    resources:
      requests:
        storage: 100Gi

  resources:
    requests:
      cpu: "2"
      memory: "4Gi"
    limits:
      cpu: "2"
      memory: "4Gi"

  aerospikeConfig:
    service:
      proto-fd-max: 15000
    namespaceDefaults:
      replication-factor: 2
      data-size: 2147483648   # 2 GiB
```

템플릿 적용:

```bash
kubectl apply -f config/samples/acko_v1alpha1_template_dev.yaml     # minimal
kubectl apply -f config/samples/acko_v1alpha1_template_stage.yaml   # soft-rack
kubectl apply -f config/samples/acko_v1alpha1_template_prod.yaml    # hard-rack
```

---

## 클러스터에서 템플릿 사용

### 기본 참조

이름으로 템플릿을 참조합니다. 템플릿이 `image`와 `size`를 제공하면 클러스터에서 해당 필드를 생략할 수 있습니다:

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: my-cluster
spec:
  templateRef:
    name: soft-rack
  aerospikeConfig:
    namespaces:
      - name: data
        storage-engine:
          type: memory
```

### 템플릿 기본값 오버라이드

클러스터에서 `spec.image`나 `spec.size`를 명시적으로 설정하여 템플릿 값을 오버라이드할 수 있습니다:

```yaml
spec:
  image: aerospike:ce-8.1.1.1   # 오버라이드: 특정 이미지 고정
  size: 3                         # 오버라이드: 템플릿의 기본값 대신 3 사용
  templateRef:
    name: hard-rack
```

---

## 템플릿 오버라이드

`spec.overrides`를 사용하여 새 템플릿을 만들지 않고도 특정 필드를 선택적으로 수정할 수 있습니다:

```yaml
spec:
  templateRef:
    name: hard-rack
  overrides:
    resources:
      requests:
        cpu: "4"
        memory: "8Gi"
      limits:
        cpu: "4"
        memory: "8Gi"
    scheduling:
      podAntiAffinityLevel: preferred   # 이 클러스터에 대해 anti-affinity 완화
```

### 병합 우선순위

필드는 다음 순서로 해석됩니다 (가장 높은 우선순위 먼저):

1. **`spec.overrides`** -- 클러스터별 오버라이드
2. **`template.spec`** -- 템플릿 기본값
3. **오퍼레이터 기본값** -- 내장 폴백 값

### 병합 동작

| 필드 유형 | 동작 | 예시 |
|-----------|------|------|
| 스칼라 (string, int, bool) | 오버라이드가 템플릿 값을 대체 | `size: 3`이 템플릿의 `size: 6`을 대체 |
| 맵 (중첩 객체) | 재귀적 병합 -- 지정된 키만 오버라이드 | `aerospikeConfig.service.proto-fd-max: 20000`은 해당 키만 오버라이드 |
| 배열 (리스트) | 오버라이드가 전체 배열을 대체 | `tolerations: [...]`이 모든 템플릿 톨러레이션을 대체 |

:::tip
`aerospikeConfig.service`와 같은 맵의 경우, 변경하려는 키만 지정하면 됩니다. 지정하지 않은 키는 템플릿에서 상속됩니다.
:::

---

## 오버라이드 가능한 필드

템플릿이 기본값을 제공할 수 있는 필드:

| 필드 | 설명 |
|------|------|
| `image` | Aerospike CE 컨테이너 이미지 |
| `size` | 기본 클러스터 크기 (1--8) |
| `resources` | CPU/메모리 요청 및 제한 |
| `scheduling` | Anti-affinity 레벨, 톨러레이션, 노드 어피니티, 토폴로지 분산 |
| `storage` | 스토리지 클래스, 볼륨 모드, 크기, 로컬 PV 플래그 |
| `rackConfig` | 랙 설정 기본값 (maxRacksPerNode) |
| `aerospikeConfig` | 서비스 및 네임스페이스 기본 설정 |
| `monitoring` | Prometheus 익스포터 사이드카 설정 |
| `aerospikeNetworkPolicy` | 클라이언트/패브릭 네트워크 접근 유형 |

---

## 템플릿 동기화 동작

### 자동 전파되지 않는 이유

템플릿은 안전을 위해 스냅샷 모델을 사용합니다. 템플릿 변경을 실행 중인 프로덕션 클러스터에 자동으로 전파하면 예기치 않은 롤링 재시작이나 설정 충돌이 발생할 수 있습니다. 대신 오퍼레이터는:

1. 템플릿 드리프트를 감지하고 `status.templateSnapshot.synced: false`를 설정합니다
2. 영향을 받는 클러스터에 `TemplateDrifted` 경고 이벤트를 발생시킵니다
3. 클러스터별로 명시적인 재동기화 승인을 대기합니다

### 동기화 상태 확인

```bash
kubectl get aerospikecluster hard-rack-cluster \
  -o jsonpath='{.status.templateSnapshot}'
```

출력 예시:

```json
{
  "name": "hard-rack",
  "resourceVersion": "12345",
  "snapshotTimestamp": "2026-03-01T10:00:00Z",
  "synced": true
}
```

`synced`가 `false`이면 클러스터가 이전 버전의 템플릿으로 실행 중입니다.

### 클러스터 재동기화

업데이트된 템플릿을 특정 클러스터에 적용하려면:

```bash
kubectl annotate aerospikecluster hard-rack-cluster acko.io/resync-template=true
```

오퍼레이터는:

1. 템플릿을 다시 가져옵니다
2. `status.templateSnapshot`을 새 스펙으로 업데이트합니다
3. `TemplateApplied` 이벤트를 발생시킵니다
4. 어노테이션을 제거합니다
5. 리컨실레이션을 트리거합니다 (설정이 변경된 경우 롤링 재시작이 발생할 수 있음)

:::warning
재동기화는 Aerospike 설정, 이미지 또는 파드 스펙에 영향을 미치는 템플릿 변경이 있는 경우 롤링 재시작을 트리거할 수 있습니다. 프로덕션 클러스터를 재동기화하기 전에 템플릿 차이를 확인하세요.
:::

---

## 템플릿 라이프사이클

### 템플릿을 사용하는 클러스터 확인

`status.usedBy` 필드는 템플릿을 참조하는 모든 클러스터를 추적합니다:

```bash
kubectl get aerospikeclustertemplate hard-rack -o jsonpath='{.status.usedBy}'
```

```json
["hard-rack-cluster", "prod-cluster-east", "prod-cluster-west"]
```

### 템플릿 삭제

`usedBy`가 비어 있지 않으면 템플릿을 삭제할 수 없습니다. 먼저 모든 클러스터 참조를 제거한 후 삭제하세요:

```bash
# 현재 참조 확인
kubectl get aerospikeclustertemplate hard-rack -o jsonpath='{.status.usedBy}'

# 클러스터에서 템플릿 참조 제거 (image/size 직접 설정)
kubectl -n aerospike patch asc hard-rack-cluster --type merge \
  -p '{"spec":{"image":"aerospike:ce-8.1.1.1","size":3,"templateRef":null}}'

# usedBy가 비어 있으면 템플릿 삭제
kubectl delete aerospikeclustertemplate hard-rack
```

### 템플릿 업데이트

다른 Kubernetes 리소스처럼 템플릿을 업데이트합니다:

```bash
kubectl patch aerospikeclustertemplate hard-rack --type merge \
  -p '{"spec":{"image":"aerospike:ce-8.1.2.0"}}'
```

업데이트 후 참조하는 모든 클러스터에 `synced: false`가 표시되고 `TemplateDrifted` 이벤트가 발생합니다. 준비가 되면 각 클러스터를 개별적으로 재동기화하세요.

---

## Helm을 통한 기본 템플릿 설치

설치 시 `defaultTemplates.enabled=true` Helm 값을 사용하여 세 가지 템플릿 티어를 모두 생성할 수 있습니다:

```bash
helm install aerospike-ce-kubernetes-operator \
  oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set defaultTemplates.enabled=true
```

확인:

```bash
kubectl get aerospikeclustertemplate
# NAME         AGE
# minimal      10s
# soft-rack    10s
# hard-rack    10s
```

---

## Cluster Manager UI를 통한 템플릿 관리

Cluster Manager UI가 활성화된 경우 (기본 활성화. K8s 관리 기능에는 `ui.k8s.enabled=true` 필요), 웹 인터페이스에서 템플릿을 관리할 수 있습니다:

- **생성** -- 모든 템플릿 필드를 위한 가이드 마법사
- **조회** -- 해석된 스펙과 참조 클러스터를 보여주는 상세 페이지
- **편집** -- 템플릿 패치/업데이트 (RBAC `patch` 및 `update` 권한 필요)
- **삭제** -- 미사용 템플릿 제거 (클러스터가 참조 중이면 차단)
- **재동기화** -- 클러스터 상세 페이지에서 템플릿 재동기화 트리거

자세한 내용은 [Cluster Manager UI](../getting-started/cluster-manager-ui.md)를 참조하세요.

---

## 모범 사례

1. **환경 티어별로 템플릿 이름 지정** -- `minimal`, `soft-rack`, `hard-rack`은 의도된 용도를 명확히 전달합니다.
2. **`description` 사용** -- `spec.description` 필드(최대 500자)는 팀이 템플릿의 목적을 이해하는 데 도움이 됩니다.
3. **프로덕션 템플릿에서 이미지 고정** -- `latest`나 태그 없는 이미지를 피하고 특정 CE 버전으로 고정하세요.
4. **프로덕션에서 Guaranteed QoS 설정** -- 예측 가능한 성능을 위해 리소스 요청을 제한과 동일하게 설정하세요.
5. **프로덕션 전에 스테이징 먼저 재동기화** -- 템플릿 업데이트 후 변경 사항을 검증하기 위해 스테이징 클러스터를 먼저 재동기화하세요.
6. **오버라이드는 최소한으로 사용** -- 많은 클러스터에 동일한 오버라이드가 필요하면 새 템플릿 티어를 만드는 것이 좋습니다.

---

## 템플릿 비교

| | `minimal` | `soft-rack` | `hard-rack` |
|---|-----------|-------------|-------------|
| **용도** | 개발 / 빠른 시작 | 스테이징 | 프로덕션 |
| **size** | 1 | 3 | 6 |
| **anti-affinity** | none | preferred | required |
| **스토리지** | standard 1 Gi | standard 10 Gi | local-path 100 Gi |
| **랙 보장** | 없음 | 소프트 (같은 노드 허용) | 하드 (1 노드 : 1 랙) |
| **리소스** | 100 m / 256 Mi | 500 m / 1 Gi | 2 / 4 Gi (Guaranteed QoS) |
| **모니터링** | 비활성화 | 비활성화 | 활성화 |
| **RF** | 1 | 2 | 2 |
