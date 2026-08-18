---
sidebar_position: 1
title: 클러스터 관리
---

# Aerospike 클러스터 관리

이 Day-2 절차로 클러스터 크기 조정, 업데이트 배포, 설정 변경, 장애 진단을 수행합니다.

## 스케일링

`spec.size`를 변경하여 클러스터를 스케일 업/다운합니다.

```bash
kubectl -n aerospike patch asc aerospike-3node --type merge -p '{"spec":{"size":5}}'
```

오퍼레이터는 클러스터가 요청한 크기에 도달할 때까지 파드를 만들거나 제거합니다. Multi-rack 배포에서는 파드를 랙마다 고르게 배치합니다.

:::warning
`spec.size`는 8을 초과할 수 없습니다 (CE 제한). `replication-factor`는 새 클러스터 크기를 초과할 수 없습니다.
:::

## 롤링 업데이트

### 이미지 업데이트

`spec.image`를 변경해 새 이미지를 rolling update합니다.

```yaml
spec:
  image: aerospike:ce-8.1.1.1   # 새 버전으로 변경
```

오퍼레이터는 `OnDelete` 업데이트 전략을 사용합니다 — 파드를 하나씩(또는 배치 단위로) 삭제하고 새 파드가 준비될 때까지 기다린 후 다음으로 진행합니다.

### 배치 크기

동시에 재시작하는 파드 수를 제어합니다:

```yaml
spec:
  rollingUpdateBatchSize: 2   # 한 번에 2개 파드 재시작 (기본값: 1)
```

Rack-aware 배포에서는 `rackConfig`에 랙별 batch size를 지정합니다. 이 값이 `spec.rollingUpdateBatchSize`보다 우선합니다.

```yaml
spec:
  rackConfig:
    rollingUpdateBatchSize: "50%"   # 랙당 파드의 50%를 동시에 재시작
```

#### 배치 크기: 정수 vs 퍼센트

배치 크기는 정수 또는 퍼센트 문자열을 허용합니다:

| 형식 | 예시 | 동작 (size=6) |
|--------|---------|-------------------|
| 정수 | `2` | 한 번에 정확히 2개 파드 |
| 퍼센트 | `"50%"` | 6의 50% = 3개 파드 |
| 퍼센트 | `"25%"` | 6의 25% = 2개 파드 (올림) |

:::tip
퍼센트 문자열은 `%` 접미사를 포함해야 합니다 (예: `"50%"`). 퍼센트는 랙당 총 파드 수에 대해 계산되며, 최소 1로 올림됩니다.
:::

#### 배치 재시작 복원력

여러 파드를 배치로 재시작할 때, 개별 파드 재시작이 실패하면 전체 배치를 중단하지 않고 다음 파드로 계속 진행합니다:

- 배치의 3개 파드 중 1개가 재시작에 실패해도 나머지 2개는 정상적으로 재시작됩니다
- 실패한 파드는 `RestartFailed` 경고 이벤트로 기록 및 보고됩니다
- 배치의 **모든** 파드가 실패한 경우에만 오류를 반환합니다
- 다음 재조정에서 성공적으로 재시작되지 않은 파드만 재시도됩니다

```bash
# 재시작 실패 확인
kubectl get events --field-selector reason=RestartFailed -n aerospike
```

### 스케일 다운 배치 크기

스케일 다운 시 랙당 동시에 제거하는 파드 수를 제어합니다:

```yaml
spec:
  rackConfig:
    scaleDownBatchSize: 2            # 랙당 한 번에 2개 파드 제거
    # scaleDownBatchSize: "25%"      # 퍼센트도 사용 가능
```

`scaleDownBatchSize`는 스케일 다운 오퍼레이션 중 **랙당** 적용됩니다. 한 번에 너무 많은 파드를 제거하여 데이터 비가용성을 초래하는 것을 방지합니다.

### 무시 가능한 파드 수

일부 파드가 Pending 또는 Failed 상태여도 reconciliation을 계속하려면 다음 값을 설정합니다.

```yaml
spec:
  rackConfig:
    maxIgnorablePods: 1   # 멈춘 파드 1개까지 무시하고 재조정 계속
```

스케줄링 문제로 파드가 멈췄을 때 전체 재조정이 차단되지 않도록 하는 데 유용합니다.

#### 배치 크기 요약

| 필드 | 범위 | 기본값 | 설명 |
|-------|-------|---------|-------------|
| `spec.rollingUpdateBatchSize` | 클러스터 전체 | 1 | 롤링 업데이트 시 동시 재시작 파드 수 |
| `rackConfig.rollingUpdateBatchSize` | 랙당 | spec에서 상속 | 랙당 클러스터 레벨 배치 크기 오버라이드 |
| `rackConfig.scaleDownBatchSize` | 랙당 | 전체 한 번에 | 스케일 다운 시 동시 제거 파드 수 |
| `rackConfig.maxIgnorablePods` | 랙당 | 0 | 재조정 중 무시할 멈춘 파드 수 |

## 설정 변경

### 정적 설정 변경

`spec.aerospikeConfig`의 모든 변경은 롤링 재시작을 트리거하여 새 설정을 적용합니다. 오퍼레이터가 각 파드의 ConfigMap에서 `aerospike.conf`를 재생성합니다.

### 동적 설정 변경

파드 재시작 없이 런타임에 설정을 변경합니다:

```yaml
spec:
  enableDynamicConfigUpdate: true
```

활성화하면, 오퍼레이터가 Aerospike의 `set-config` 명령을 사용하여 가능한 경우 런타임에 설정 변경을 적용합니다. 동적으로 적용할 수 없는 변경만 롤링 재시작을 트리거합니다.

#### 동적으로 변경 가능한 설정

대부분의 Aerospike service 및 namespace 파라미터는 동적으로 변경 가능합니다. 예시:

| 카테고리 | 동적 파라미터 |
|----------|--------------|
| Service | `proto-fd-max`, `transaction-pending-limit`, `batch-max-buffers-per-queue` |
| Namespace | `high-water-memory-pct`, `high-water-disk-pct`, `stop-writes-pct`, `nsup-period`, `default-ttl` |
| 동적 아님 | `replication-factor`, `storage-engine type`, `name` (재시작 필요) |

#### 부분 실패 시 롤백

여러 동적 설정 변경을 적용할 때, 오퍼레이터는 성공적으로 적용된 각 변경을 추적합니다. 중간에 변경이 실패하면:

1. 오퍼레이터가 이전에 적용된 모든 변경을 원래 값으로 **역순 롤백**합니다
2. 롤백은 최선 노력(best-effort)입니다 — 롤백 명령도 실패하면 로그에 기록하지만 진행을 차단하지 않습니다
3. 롤백 후 올바른 설정을 원자적으로 적용하기 위해 **콜드 리스타트**로 폴백합니다

이 롤백은 일부 설정만 적용된 채 클러스터가 실행되는 상황을 막습니다. 오퍼레이터 로그에서 동작을 확인합니다.

```bash
kubectl -n aerospike-operator logs -l control-plane=controller-manager | grep -i "rollback\|dynamic config"
```

#### 사전 검증

동적 변경을 적용하기 전, 오퍼레이터는 모든 변경을 배치로 검증합니다. 파라미터 값에 잘못된 문자(예: `;` 또는 `:`)가 포함되어 있으면 `set-config` 명령이 전송되기 전에 전체 배치가 거부됩니다. 이를 통해 명백히 잘못된 입력으로 인한 부분 적용을 방지합니다.

#### dynamicConfigStatus 확인

`enableDynamicConfigUpdate: true`로 설정 변경 후 파드별 상태를 확인합니다:

```bash
kubectl -n aerospike get asc aerospike-3node -o jsonpath='{.status.pods}' | jq '.[] | {name: .podName, dynamicConfig: .dynamicConfigStatus}'
```

| 상태 | 의미 |
|--------|---------|
| `Applied` | 런타임에 동적 설정이 성공적으로 적용됨 |
| `Failed` | 동적 업데이트 실패 — 롤링 리스타트가 트리거됨 |
| `Pending` | 오퍼레이터의 변경 적용 대기 중 |
| (비어있음) | 동적 설정 변경이 시도되지 않음 |

## Pod Readiness Gates

기본적으로 파드는 Kubernetes가 `PodReady=True`를 보고할 때 "준비됨"으로 간주됩니다. 이는 Aerospike가 클러스터 mesh에 완전히 참여하고 데이터 마이그레이션을 완료하기 전에 파드가 Service 엔드포인트에 추가될 수 있음을 의미합니다.

커스텀 readiness gate `acko.io/aerospike-ready`를 활성화하여 Aerospike가 진정으로 준비될 때까지 각 파드를 Service 엔드포인트에서 제외합니다:

```yaml
spec:
  podSpec:
    readinessGateEnabled: true
```

활성화하면 오퍼레이터가:
1. 모든 파드의 `spec.readinessGates`에 `acko.io/aerospike-ready`를 주입합니다
2. 다음 조건이 충족된 후에만 파드의 `status.conditions`를 `True`로 패치합니다:
   - 파드의 Aerospike 프로세스가 클러스터 mesh에 참여했을 때, **그리고**
   - 모든 데이터 마이그레이션이 완료되었을 때 (`cluster-stable: true`)
3. 롤링 리스타트를 보류합니다 — 이전 파드의 게이트가 충족될 때까지 다음 파드를 삭제하지 않습니다

:::info
`readinessGateEnabled` 변경은 `ReadinessGates`가 파드 생성 후 불변이므로 롤링 리스타트를 트리거합니다. 오퍼레이터가 이를 자동으로 처리합니다.
:::

:::note
이것은 **옵트인** 기능입니다. `readinessGateEnabled`가 설정되지 않은(또는 `false`인) 기존 클러스터는 이전과 동일하게 동작합니다.
:::

### 게이트 상태 확인

파드별 게이트 조건을 확인합니다:

```bash
kubectl -n aerospike get pod aerospike-3node-0 \
  -o jsonpath='{.status.conditions}' | jq '.[] | select(.type=="acko.io/aerospike-ready")'
```

오퍼레이터는 클러스터의 파드 상태에 게이트 상태를 반영합니다:

```bash
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='{.status.pods}' | jq 'to_entries[] | {pod: .key, gateOk: .value.readinessGateSatisfied}'
```

### Readiness Gates와 롤링 리스타트 동작

Readiness gates 없이:
```
Pod-2 삭제 → Pod-2 Running → Pod-1 삭제 → ...   (K8s Ready = 충분)
```

`readinessGateEnabled: true`일 때:
```
Pod-2 삭제 → Pod-2 Running → Gate=False (마이그레이션 중) → Gate=True → Pod-1 삭제 → ...
```

게이트 대기로 리스타트가 차단되면 `ReadinessGateBlocking` 경고 이벤트가 발생합니다:

```bash
kubectl -n aerospike get events --field-selector reason=ReadinessGateBlocking
```

## 마이그레이션 상태 모니터링

오퍼레이터는 `status.migrationStatus`에서 데이터 마이그레이션 진행 상태를 추적합니다. 각 재조정 시 모든 Aerospike 노드의 `migrate_partitions_remaining` 통계를 조회하여 결과를 집계합니다.

**마이그레이션이 발생하는 시점:**

- **스케일 업/다운** — 노드 추가 또는 제거 시 파티션 리밸런싱 발생.
- **롤링 리스타트** — 재시작된 각 노드가 파티션 사본을 다시 수신해야 함.
- **랙 변경** — 랙 간 파드 이동 시 데이터 재분배.
- **Replication factor 변경** — RF 증가 시 새 복제본 생성.

**마이그레이션 상태 확인:**

```bash
# 클러스터 레벨 마이그레이션 상태
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='{.status.migrationStatus}' | jq .
```

출력 예시:
```json
{
  "inProgress": true,
  "remainingPartitions": 142857,
  "lastChecked": "2026-03-13T10:30:00Z"
}
```

**파드별 마이그레이션 레코드:**

```bash
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='{.status.pods}' | jq 'to_entries[] | {pod: .key, migrating: .value.migratingPartitions}'
```

**jsonpath로 빠르게 확인:**

```bash
# 마이그레이션 진행 중인지 확인 (true/false 반환)
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='InProgress={.status.migrationStatus.inProgress} Remaining={.status.migrationStatus.remainingPartitions}'

# MigrationComplete 조건 직접 확인
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='{range .status.conditions[?(@.type=="MigrationComplete")]}{.status}{end}'
```

**Prometheus 메트릭:**

오퍼레이터는 `acko_cluster_migrating_partitions`를 `namespace`와 `name` 레이블이 있는 Prometheus 게이지 메트릭으로 노출합니다. 장기 마이그레이션에 대한 알림 설정이 가능합니다:

```promql
# 30분 이상 마이그레이션이 진행 중일 때 알림
acko_cluster_migrating_partitions{namespace="aerospike", name="aerospike-3node"} > 0

# 마이그레이션 진행 속도 추적 (초당 마이그레이션된 파티션)
deriv(acko_cluster_migrating_partitions[5m])

# 멈춘 마이그레이션 알림 (남은 파티션이 줄어들지 않음)
deriv(acko_cluster_migrating_partitions[10m]) >= 0
  and acko_cluster_migrating_partitions > 0
```

:::tip
`status.conditions`의 `MigrationComplete` 조건은 `migrationStatus.remainingPartitions`가 0에 도달하면 `True`로 설정됩니다. 전체 마이그레이션 상태를 파싱하지 않고 간단한 상태 확인에 활용하세요.
:::

## 클러스터 상태 및 Conditions

오퍼레이터는 `status` subresource에 상세 상태를 기록합니다. 모니터링과 트러블슈팅에서는 아래 필드를 먼저 확인하세요.

### Phase

`status.phase`는 오퍼레이터가 수행 중인 작업을 요약합니다.

```bash
kubectl -n aerospike get asc
```

| Phase | 의미 |
|---|---|
| `Completed` | 클러스터가 정상이며 원하는 스펙과 일치 |
| `InProgress` | 일반적인 재조정 진행 중 |
| `ScalingUp` | 클러스터에 파드 추가 중 |
| `ScalingDown` | 클러스터에서 파드 제거 중 |
| `WaitingForMigration` | 데이터 마이그레이션 완료까지 스케일 다운 보류 |
| `RollingRestart` | 롤링 리스타트 진행 중 (설정/이미지/파드스펙 변경) |
| `ACLSync` | ACL 역할 및 사용자 동기화 중 |
| `Paused` | 사용자에 의해 재조정 일시 중지됨 |
| `Deleting` | 클러스터 해체 진행 중 |
| `Error` | 복구 불가능한 오류; `status.lastReconcileError` 확인 |

`status.phaseReason`은 원인을 더 구체적으로 설명합니다(예: "Rolling restart in progress for rack 1").

### Health

`status.health`는 "준비됨/전체" 형식으로 상태를 요약합니다.

```bash
kubectl -n aerospike get asc aerospike-3node -o jsonpath='{.status.health}'
# 출력: 3/3
```

`2/3` 값은 3개 파드 중 2개가 준비되었음을 의미합니다. `kubectl get asc` 출력의 `HEALTH` 컬럼에 매핑됩니다.

### Conditions

오퍼레이터는 클러스터 상태의 상세한 분석을 제공하는 6가지 조건 유형을 유지합니다:

```bash
kubectl -n aerospike get asc aerospike-3node -o jsonpath='{.status.conditions}' | jq .
```

| Condition | True인 경우 |
|---|---|
| `Available` | 최소 하나의 파드가 요청을 처리할 준비됨 |
| `Ready` | 모든 원하는 파드가 실행 중이며 준비됨 |
| `ConfigApplied` | 모든 파드에 원하는 Aerospike 설정이 적용됨 |
| `ACLSynced` | ACL 역할과 사용자가 스펙과 일치 |
| `MigrationComplete` | 보류 중인 데이터 마이그레이션이 없음 |
| `ReconciliationPaused` | `spec.paused`가 `true` |

**예시: 클러스터가 완전히 정상인지 확인**

`Ready`, `ConfigApplied`, `MigrationComplete`, `ACLSynced`(ACL 설정된 경우)가 모두 `True`이면 완전히 정상입니다:

```bash
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='{range .status.conditions[*]}{.type}={.status}{"\n"}{end}'
```

예상 정상 출력:
```
Available=True
Ready=True
ConfigApplied=True
ACLSynced=True
MigrationComplete=True
ReconciliationPaused=False
```

### Aerospike 클러스터 크기 vs Kubernetes 파드 수

`status.aerospikeClusterSize` 필드는 Aerospike의 `asinfo` 명령이 보고하는 클러스터 크기를 반영합니다. 다음 상황에서 `status.size`(준비된 Kubernetes 파드 수)와 일시적으로 다를 수 있습니다:

- 롤링 리스타트 (파드가 교체되는 중)
- 네트워크 파티션 (split-brain 시나리오)
- 파드 시작 (Aerospike가 아직 mesh에 참여하지 않음)

```bash
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='K8s pods: {.status.size}, Aerospike cluster-size: {.status.aerospikeClusterSize}'
```

이 값들이 장시간 다르면 파드 연결성과 mesh heartbeat 설정을 조사하세요.

## Secret 트리거 ACL 동기화

오퍼레이터는 `aerospikeAccessControl.users[*].secretName`으로 참조되는 Kubernetes Secret을 감시합니다. Secret의 데이터가 변경되면(예: 비밀번호 교체) AerospikeCluster CR에 대한 변경 없이 오퍼레이터가 자동으로 재조정을 트리거하여 업데이트된 비밀번호를 Aerospike에 동기화합니다.

다음과 같이 별도 조작 없이 비밀번호를 교체할 수 있습니다.

```bash
# Secret 업데이트로 사용자 비밀번호 교체
kubectl -n aerospike create secret generic app-secret \
  --from-literal=password='new-password-here' \
  --dry-run=client -o yaml | kubectl apply -f -
```

오퍼레이터는 Secret 변경을 감지해 ACL을 동기화하고 Aerospike 사용자 비밀번호를 갱신합니다. 이벤트로 결과를 확인합니다.

```bash
kubectl get events --field-selector reason=ACLSyncStarted -n aerospike
kubectl get events --field-selector reason=ACLSyncCompleted -n aerospike
```

:::info
AerospikeCluster의 ACL 설정에서 실제로 참조하는 Secret만 재조정을 트리거합니다. 동일 네임스페이스의 관련 없는 Secret 변경은 무시됩니다.
:::

## 재조정 일시 중지 및 재개

`spec.paused: true`를 설정하면 오퍼레이터가 클러스터 reconciliation을 잠시 멈춥니다. 이 상태에서는 scaling, rolling restart, 설정 변경, ACL sync를 수행하지 않습니다. 클러스터 phase는 `Paused`, `ReconciliationPaused` condition은 `True`가 됩니다.

**일시 중지가 필요한 경우:**

- 계획된 인프라 유지보수 중 (노드 업그레이드, 스토리지 마이그레이션)
- 수동 디버깅 중 오퍼레이터의 간섭 방지
- 단일 배치로 적용하려는 여러 스펙 변경 전
- 오퍼레이터의 재시도 없이 멈춘 클러스터를 조사할 때

```bash
# 재조정 일시 중지
kubectl -n aerospike patch asc aerospike-3node --type merge -p '{"spec":{"paused":true}}'

# 일시 중지 상태 확인
kubectl -n aerospike get asc aerospike-3node -o jsonpath='{.status.phase}'
# 출력: Paused
```

재조정을 재개하려면 `paused`를 `false`로 설정하거나 완전히 제거합니다. 오퍼레이터가 즉시 원하는 상태를 향해 클러스터를 재조정하기 시작합니다:

```bash
# 재조정 재개
kubectl -n aerospike patch asc aerospike-3node --type merge -p '{"spec":{"paused":null}}'
```

:::warning
일시 중지는 Aerospike 클러스터 자체를 중지하지 않습니다 — 파드는 계속 실행되며 트래픽을 처리합니다. 오퍼레이터가 클러스터의 Kubernetes 리소스를 변경하는 것만 중지됩니다.
:::

## 온디맨드 오퍼레이션

`spec.operations`를 통해 파드 재시작을 선언적으로 트리거합니다. 한 번에 하나의 오퍼레이션만 활성화할 수 있습니다.

### WarmRestart

Aerospike 프로세스에 SIGUSR1을 보내 파드 삭제 없이 graceful restart합니다:

```yaml
spec:
  operations:
    - kind: WarmRestart
      id: "config-reload-v2"       # 고유 ID (1-20자)
      podList:                      # 선택사항: 비우면 전체 파드 대상
        - aerospike-3node-0
        - aerospike-3node-1
```

### PodRestart

파드를 삭제하고 재생성합니다 (cold restart):

```yaml
spec:
  operations:
    - kind: PodRestart
      id: "cold-restart-01"
      podList:
        - aerospike-3node-2
```

### 오퍼레이션 상태 확인

```bash
kubectl -n aerospike get asc aerospike-3node -o jsonpath='{.status.operationStatus}' | jq .
```

상태에는 `phase` (`InProgress`, `Completed`, `Error`), `completedPods`, `failedPods`가 포함됩니다.

:::warning
- InProgress 중에는 오퍼레이션을 변경할 수 없습니다
- 오퍼레이션 `id`는 고유해야 합니다 (1-20자)
- 완료 후 spec에서 오퍼레이션을 제거하세요
:::

## 서비스 커스터마이징

### Headless 서비스 메타데이터

Headless 서비스(파드 디스커버리용)에 커스텀 어노테이션과 레이블을 추가합니다:

```yaml
spec:
  headlessService:
    metadata:
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8687"
      labels:
        monitoring: enabled
```

### Pod별 서비스

`podService`를 설정하면 오퍼레이터가 각 파드에 대해 개별 ClusterIP Service를 생성하여 직접적인 파드 레벨 접근을 가능하게 합니다:

```yaml
spec:
  podService:
    metadata:
      annotations:
        external-dns.alpha.kubernetes.io/hostname: "aero.example.com"
      labels:
        service-type: pod-local
```

**사용 사례:** 외부 DNS 연동, 파드 레벨 로드밸런싱, 특정 파드에 대한 직접 클라이언트 접근.

## 검증 정책

웹훅 검증 동작을 제어합니다:

```yaml
spec:
  validationPolicy:
    skipWorkDirValidate: true   # 작업 디렉토리 PV 검증 건너뛰기
```

| 필드 | 기본값 | 설명 |
|---|---|---|
| `skipWorkDirValidate` | `false` | 작업 디렉토리가 영구 스토리지에 있는지 검증을 건너뜁니다 |

개발 환경이나 영구 작업 디렉토리가 필요 없는 인메모리 배포에 유용합니다.

## 스토리지

### 볼륨 타입

| 소스 | 사용 사례 |
|---|---|
| `persistentVolume` | 영구 데이터 (파드 재시작 시 유지) |
| `emptyDir` | 임시 작업 공간 |
| `secret` | 자격 증명, TLS 인증서 |
| `configMap` | 커스텀 설정 파일 |
| `hostPath` | 노드 로컬 경로 (개발/테스트 전용) |

### 글로벌 볼륨 정책

카테고리별(파일시스템 또는 블록) 모든 영구 볼륨의 기본 정책을 설정합니다. 볼륨별 설정이 항상 글로벌 정책보다 우선합니다.

```yaml
storage:
  filesystemVolumePolicy:
    initMethod: deleteFiles
    wipeMethod: deleteFiles
    cascadeDelete: true
  blockVolumePolicy:
    initMethod: blkdiscard
    wipeMethod: blkdiscardWithHeaderCleanup
```

오퍼레이터는 다음 우선순위로 설정을 결정합니다:
1. **볼륨별** `initMethod` / `wipeMethod` / `cascadeDelete`
2. **글로벌 정책** (`volumeMode` 기반: `filesystemVolumePolicy` 또는 `blockVolumePolicy`)
3. **기본값**: `none` / `none` / `false`

### Cascade Delete

`cascadeDelete: true`이면 AerospikeCluster CR 삭제 시 PVC가 자동으로 삭제됩니다. 볼륨별 또는 글로벌 정책으로 설정할 수 있습니다.

```yaml
storage:
  # 글로벌: CR 삭제 시 모든 파일시스템 PVC 삭제
  filesystemVolumePolicy:
    cascadeDelete: true
  volumes:
    - name: data-vol
      source:
        persistentVolume:
          storageClass: standard
          size: 10Gi
      # filesystemVolumePolicy에서 cascadeDelete: true 상속
```

#### 스케일 다운 시 PVC 정리 안전성

스케일 다운 중 오퍼레이터는 PVC를 삭제하기 전에 모든 스케일 다운된 파드가 완전히 종료되었는지 확인합니다. 이는 파드의 Aerospike 프로세스가 아직 데이터를 쓰는 중에 PVC가 삭제되는 경합 조건을 방지합니다.

스케일 다운된 파드가 아직 실행 중이거나 종료 중이면 PVC 정리는 다음 재조정 사이클로 **지연**됩니다. 오퍼레이터는 다음과 같이 로그를 남깁니다:

```
Deferring PVC cleanup: scaled-down pods still terminating
```

모든 스케일 다운된 파드가 종료 확인되면 오퍼레이터는 `cascadeDelete: true`인 볼륨의 PVC만 삭제합니다. 비 cascade PVC는 항상 보존됩니다.

PVC 정리 과정은 이벤트로 확인합니다.

```bash
# 성공적인 정리
kubectl get events --field-selector reason=PVCCleanedUp -n aerospike

# 실패한 정리
kubectl get events --field-selector reason=PVCCleanupFailed -n aerospike
```

### 볼륨 초기화 방법

| 방법 | 설명 |
|---|---|
| `none` | 초기화 없음 (기본값) |
| `deleteFiles` | 볼륨의 모든 파일 삭제 |
| `dd` | `dd`로 볼륨 제로 채우기 |
| `blkdiscard` | 블록 디스카드 (블록 디바이스 전용) |
| `headerCleanup` | Aerospike 파일 헤더만 정리 |

```yaml
storage:
  volumes:
    - name: data-vol
      initMethod: deleteFiles
```

### 와이프 방법

와이프 방법은 초기화 방법과 유사하지만 **더티 볼륨** (비정상 종료 후 정리가 필요한 볼륨)에 적용됩니다. `wipeMethod` 필드는 다음 값을 지원합니다:

| 방법 | 설명 |
|---|---|
| `none` | 와이프 없음 (기본값) |
| `deleteFiles` | 볼륨의 모든 파일 삭제 |
| `dd` | `dd`를 사용하여 디바이스를 제로 필 |
| `blkdiscard` | 디바이스의 모든 블록 디스카드 |
| `headerCleanup` | Aerospike 파일 헤더만 정리 |
| `blkdiscardWithHeaderCleanup` | 블록 디스카드 후 Aerospike 헤더 정리 |

```yaml
storage:
  volumes:
    - name: data-vol
      wipeMethod: headerCleanup
```

### HostPath 볼륨

:::warning
HostPath 볼륨은 **프로덕션에 권장되지 않습니다**. 데이터가 특정 노드에 종속되어 파드 재스케줄링 시 이동이 불가능합니다.
:::

```yaml
storage:
  volumes:
    - name: host-logs
      source:
        hostPath:
          path: /var/log/aerospike
          type: DirectoryOrCreate
      aerospike:
        path: /opt/aerospike/logs
```

### PVC 커스텀 메타데이터

PersistentVolumeClaim에 커스텀 라벨과 어노테이션을 추가합니다:

```yaml
storage:
  volumes:
    - name: data-vol
      source:
        persistentVolume:
          storageClass: standard
          size: 50Gi
          metadata:
            labels:
              backup-policy: "daily"
            annotations:
              volume.kubernetes.io/storage-provisioner: "ebs.csi.aws.com"
```

### PVC 관리

오퍼레이터는 각 파드의 스토리지 볼륨에 대해 PersistentVolumeClaim을 생성합니다. 클러스터 매니저 UI를 통해 PVC 상태를 모니터링할 수 있으며, 다음 정보를 표시합니다:

- **PVC 바인딩 상태** -- Bound, Pending, Released, Failed
- **파드 바인딩** -- 각 PVC가 현재 바인딩된 파드
- **고아 PVC** -- PersistentVolume에 바인딩되어 있지만 실행 중인 파드에 마운트되지 않은 PVC (스케일 다운 또는 파드 재스케줄링 후 발생 가능)
- **용량 및 StorageClass** -- 프로비저닝된 스토리지 크기와 사용된 StorageClass

`kubectl`로 PVC 상태를 확인합니다.

```bash
# 클러스터의 모든 PVC 조회
kubectl -n aerospike get pvc -l app=aerospike-ce-3node

# 고아 PVC 확인 (매칭되는 파드가 없는 PVC)
kubectl -n aerospike get pvc -l app=aerospike-ce-3node -o custom-columns=NAME:.metadata.name,STATUS:.status.phase,POD:.metadata.annotations.acko\\.io/pod-name
```

:::note
`cascadeDelete: true`인 PVC는 CR 삭제 시 또는 스케일 다운 후 자동으로 정리됩니다. 자세한 내용은 [Cascade Delete](#cascade-delete) 섹션을 참조하세요.
:::

### 볼륨 마운트 옵션

Aerospike 및 사이드카 컨테이너의 고급 마운트 옵션:

```yaml
storage:
  volumes:
    - name: shared-data
      source:
        emptyDir: {}
      aerospike:
        path: /opt/aerospike/shared
        readOnly: false
        subPath: "aerospike-data"       # 하위 디렉토리 마운트
        mountPropagation: HostToContainer
      sidecars:
        - containerName: exporter
          path: /shared
          readOnly: true
```

| 옵션 | 설명 |
|---|---|
| `readOnly` | 볼륨을 읽기 전용으로 마운트 |
| `subPath` | 볼륨의 특정 하위 디렉토리 마운트 |
| `subPathExpr` | subPath와 유사하나 환경 변수 확장 지원 |
| `mountPropagation` | 마운트 전파 제어 (`None`, `HostToContainer`, `Bidirectional`) |

:::note
`subPath`와 `subPathExpr`는 상호 배타적입니다.
:::

### 로컬 스토리지

파드 재시작 시 특수 처리를 위해 스토리지 클래스를 로컬로 표시합니다:

```yaml
storage:
  localStorageClasses:
    - local-path
    - openebs-hostpath
  deleteLocalStorageOnRestart: true
```

`deleteLocalStorageOnRestart: true`이면 오퍼레이터는 콜드 리스타트 시 파드 삭제 **전에** 로컬 스토리지 클래스로 지원되는 PVC를 삭제합니다. 로컬 스토리지는 노드에 종속되므로 새 노드에서 재프로비저닝이 필요합니다.

## 네트워크 설정

### 접근 타입

클라이언트가 Aerospike 노드를 발견하고 연결하는 방법을 제어합니다:

| 타입 | 설명 |
|---|---|
| `pod` | 파드 IP (기본값, 클러스터 내부 클라이언트) |
| `hostInternal` | 노드 내부 IP |
| `hostExternal` | 노드 외부 IP |
| `configuredIP` | 파드 어노테이션의 커스텀 IP |

```yaml
spec:
  aerospikeNetworkPolicy:
    accessType: pod
    alternateAccessType: hostExternal
    fabricType: pod
```

### LoadBalancer

LoadBalancer 서비스로 클러스터를 노출합니다:

```yaml
spec:
  seedsFinderServices:
    loadBalancer:
      port: 3000
      targetPort: 3000
      annotations:
        service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
      loadBalancerSourceRanges:
        - "10.0.0.0/8"
```

### NetworkPolicy

자동 NetworkPolicy 생성을 활성화합니다:

```yaml
spec:
  networkPolicyConfig:
    enabled: true
    type: kubernetes    # 또는 CiliumNetworkPolicy용 "cilium"
```

### 대역폭 제한

CNI 어노테이션을 통한 대역폭 제한 설정 (예: Cilium):

```yaml
spec:
  bandwidthConfig:
    ingress: "1Gbps"
    egress: "1Gbps"
```

## HorizontalPodAutoscaler (HPA)

AerospikeCluster는 `scale` 서브리소스를 지원하여 Kubernetes HPA와 통합할 수 있습니다. 오퍼레이터는 HPA 호환성을 위해 `status.selector`와 `status.replicas`를 노출합니다.

:::note
서브리소스가 읽는 값은 셀렉터에 매칭되는 전체 pod 수인 `status.replicas`이며, ready pod만 세는 `status.size`가 아닙니다. rolling restart 중에는 `status.size`가 실제 replica 수보다 낮아지므로, 그 값을 HPA에 넘기면 정상 클러스터가 scale down 됩니다. readiness는 `status.size` / `status.health`로, replica 수는 `status.replicas`로 확인하세요.
:::

### HPA 생성

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: aerospike-hpa
  namespace: aerospike
spec:
  scaleTargetRef:
    apiVersion: acko.io/v1alpha1
    kind: AerospikeCluster
    name: aerospike-3node
  minReplicas: 2
  maxReplicas: 8    # CE 최대값
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: 80
```

```bash
kubectl apply -f hpa.yaml
```

### HPA 상태 확인

```bash
kubectl -n aerospike get hpa aerospike-hpa
```

:::warning
CE 에디션은 최대 클러스터 크기가 8 노드입니다. `maxReplicas`를 8 이하로 설정하세요.
HPA는 `spec.size`를 스케일링하며, 이는 오퍼레이터의 일반적인 스케일링 로직(랙 인식 분배, 마이그레이션 인식 스케일 다운)을 트리거합니다.
:::

:::note
HPA를 사용할 때는 `spec.size`를 수동으로 변경하지 마세요 — 오토스케일러가 관리하도록 합니다. 일시적으로 오버라이드해야 하면 먼저 HPA를 일시 중지하세요.
:::

---

## Pod Disruption Budget

기본적으로 오퍼레이터는 유지보수 중 클러스터를 보호하기 위해 PodDisruptionBudget을 생성합니다.

### rack별 PDB

| 토폴로지 | 생성되는 PDB |
|---|---|
| 단일 rack (`rackConfig` 없음 또는 rack 1개) | 클러스터 전체 PDB 1개, `<cluster>-pdb` |
| Multi-rack | rack별 PDB, `<cluster>-<rackID>-pdb` |

클러스터 전체 PDB 하나는 모든 rack에 걸쳐 disruption 수를 셉니다. rack 3개 × pod 2개에 `maxUnavailable: 1`이면 Kubernetes는 한 번에 1개 eviction만 허용하지만, *어떤* pod인지는 제약하지 않으므로 drain이 같은 rack의 pod 2개를 연달아 제거해 해당 rack을 비울 수 있습니다. rack별 PDB는 이 제약을 rack 단위로 만듭니다.

spec에서 제거된 rack의 PDB는 삭제되며, 단일 rack ↔ multi-rack 전환 시 두 형태 사이를 오갑니다.

### 기본값: rack의 과반이 유지됨

`maxUnavailable`를 설정하지 않으면 각 PDB는 `minAvailable = rackSize/2 + 1`을 사용합니다.

| rack 크기 | minAvailable | 허용되는 eviction |
|---|---|---|
| 1 | 1 | 0 |
| 2 | 2 | 0 |
| 3 | 2 | 1 |
| 4 | 3 | 1 |
| 5 | 3 | 2 |
| 6 | 4 | 2 |

:::warning rack이 1~2개 pod이면 voluntary disruption이 전혀 허용되지 않습니다
이 크기에서는 과반이 곧 전체이므로, 해당 pod이 있는 노드의 `kubectl drain`은 `maxUnavailable`를 명시하거나 rack을 키우거나 `spec.disablePDB: true`를 설정할 때까지 차단됩니다. 이는 과반 규칙의 직접적인 결과이며 버그가 아닙니다.
:::

우선순위: `rack.maxUnavailable` > `spec.maxUnavailable` > 과반 기본값.

보호 대상 pod을 **전부** eviction 할 수 있게 하는 값(3-pod rack에 `maxUnavailable: 3`, 또는 100% 이상의 퍼센트)은 admission에서 거부됩니다. 그것은 budget이 아닙니다. disruption 보호를 끄려면 `spec.disablePDB: true`를 명시적으로 사용하세요.

### PDB 비활성화

```yaml
spec:
  disablePDB: true
```

### 커스텀 MaxUnavailable

```yaml
spec:
  maxUnavailable: 1         # 정수 또는 "25%" 같은 퍼센트 문자열 가능
```

## 호스트 네트워크

직접 노드 포트 접근을 위한 호스트 네트워킹 활성화:

```yaml
spec:
  podSpec:
    hostNetwork: true
    # 자동으로 적용되는 기본값:
    #   multiPodPerHost: false
    #   dnsPolicy: ClusterFirstWithHostNet
```

## 랙 라벨 스케줄링

`rackLabel`을 사용하여 특정 라벨이 있는 노드에 파드를 스케줄링합니다. 오퍼레이터가 `acko.io/rack=<rackLabel>`에 대한 노드 어피니티를 설정합니다:

```yaml
spec:
  rackConfig:
    racks:
      - id: 1
        rackLabel: zone-a    # acko.io/rack=zone-a 노드에 스케줄링
      - id: 2
        rackLabel: zone-b
      - id: 3
        rackLabel: zone-c
```

:::warning
`rackLabel` 값은 랙 간에 고유해야 합니다.
:::

제어된 마이그레이션을 위해 각 랙에 `revision`을 부여할 수도 있습니다:

```yaml
spec:
  rackConfig:
    racks:
      - id: 1
        rackLabel: zone-a
        revision: "v1.0"     # 마이그레이션 추적용 버전 식별자
```

## 노드 스케줄링

### 노드 셀렉터

```yaml
spec:
  podSpec:
    nodeSelector:
      node-type: aerospike
```

### Tolerations

```yaml
spec:
  podSpec:
    tolerations:
      - key: "dedicated"
        operator: "Equal"
        value: "aerospike"
        effect: "NoSchedule"
```

### 노드 블록 리스트

특정 노드에서 스케줄링을 방지합니다:

```yaml
spec:
  k8sNodeBlockList:
    - node-maintenance-01
    - node-maintenance-02
```

## 트러블슈팅

### 클러스터 Phase 확인

```bash
kubectl -n aerospike get asc
```

| Phase | 의미 |
|---|---|
| `InProgress` | 재조정 진행 중 (일반) |
| `Completed` | 클러스터 정상, 최신 상태 |
| `Error` | 재조정 중 복구 불가능한 오류 발생 |
| `ScalingUp` | 클러스터 스케일 업 중 (파드 추가) |
| `ScalingDown` | 클러스터 스케일 다운 중 (파드 제거) |
| `RollingRestart` | 롤링 리스타트 진행 중 |
| `ACLSync` | ACL 역할 및 사용자 동기화 중 |
| `Paused` | 사용자에 의해 재조정 일시 중지됨 |
| `Deleting` | 클러스터 삭제 중 |

### Conditions 확인

```bash
kubectl -n aerospike get asc aerospike-3node -o jsonpath='{.status.conditions}' | jq .
```

### 파드 상태 확인

```bash
kubectl -n aerospike get asc aerospike-3node -o jsonpath='{.status.pods}' | jq .
```

각 파드 상태에 포함된 정보:

| 필드 | 설명 |
|---|---|
| `podIP` | 파드 IP 주소 |
| `hostIP` | 노드 IP 주소 |
| `image` | 실행 중인 컨테이너 이미지 |
| `rack` | 랙 ID |
| `isRunningAndReady` | 파드 정상 여부 |
| `configHash` | 적용된 설정의 SHA256 해시 |
| `dynamicConfigStatus` | `Applied`, `Failed`, `Pending`, 또는 빈 문자열 |
| `nodeID` | Aerospike가 할당한 노드 식별자 (예: `BB9020012AC4202`) |
| `clusterName` | 노드가 보고한 Aerospike 클러스터 이름 |
| `accessEndpoints` | 직접 클라이언트 접근용 네트워크 엔드포인트 |
| `readinessGateSatisfied` | `acko.io/aerospike-ready` 게이트 충족 시 `true` (`readinessGateEnabled: true` 필요) |
| `lastRestartReason` | 파드가 마지막으로 재시작된 이유: `ConfigChanged`, `ImageChanged`, `PodSpecChanged`, `ManualRestart`, `WarmRestart` |
| `lastRestartTime` | 오퍼레이터에 의한 마지막 재시작 타임스탬프 |
| `unstableSince` | 이 파드가 처음 NotReady가 된 시점; Ready 시 초기화 |
| `migratingPartitions` | 이 파드가 현재 마이그레이션 중인 파티션 수; 접근 불가 시 `nil` |

### 오퍼레이터 로그 확인

```bash
kubectl -n aerospike-operator logs -l control-plane=controller-manager -f
```

### 서킷 브레이커와 지수 백오프

오퍼레이터는 지속적으로 실패하는 클러스터에 대한 과도한 재시도를 방지하기 위해 내장 서킷 브레이커를 포함합니다. **10회 연속 재조정 실패** 후 오퍼레이터가 백오프 상태에 진입합니다:

| 연속 실패 횟수 | 백오프 지연 |
|---------------------|---------------|
| 1 | 2초 |
| 2 | 4초 |
| 3 | 8초 |
| 5 | 32초 |
| 8+ | ~4.3분 (256초 상한) |

서킷 브레이커가 활성화된 동안 `CircuitBreakerActive` 경고 이벤트가 실패 횟수 및 마지막 오류와 함께 발생합니다. 재조정이 성공하면 카운터가 리셋되고 `CircuitBreakerReset` 이벤트가 발생합니다.

```bash
# 서킷 브레이커 활성 여부 확인
kubectl get events --field-selector reason=CircuitBreakerActive -n aerospike

# 실패 횟수와 마지막 오류 확인
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='{.status.failedReconcileCount}{"\t"}{.status.lastReconcileError}'
```

:::info
**영구적 검증 오류**(예: 잘못된 Aerospike 설정 구조, ACL 시크릿 누락, 잘못된 권한 코드)는 `failedReconcileCount`를 최대 임계값으로 즉시 설정하여 서킷 브레이커를 **즉시 활성화**합니다. `PermanentError` 이벤트가 발생하고, `ReconcileHealthy` 상태 조건이 `False` (reason: `PermanentError`)로 설정됩니다. 이러한 오류는 자동으로 복구되지 않으므로 spec을 수정해야 합니다.
:::

### 일반적인 문제

**Phase가 InProgress에서 멈춤:**
- 오퍼레이터 로그에서 오류 세부사항 확인
- 스토리지 클래스 존재 여부 확인: `kubectl get sc`
- 이미지 풀 가능 여부 확인: `kubectl -n aerospike describe pod <파드-이름>`

**파드 CrashLoopBackOff:**
- Aerospike 로그 확인: `kubectl -n aerospike logs <파드-이름> -c aerospike`
- `aerospikeConfig`가 유효한지 확인 (네임스페이스 이름, 스토리지 경로)

**웹훅 거부:**
- 오류 메시지를 읽으세요 — 웹훅이 CE 제약 조건을 검증합니다
- [CE 검증 규칙](../getting-started/create-cluster#ce-검증-규칙)을 확인하세요

## Kubernetes 이벤트

오퍼레이터는 모든 중요한 라이프사이클 전환에 대해 Kubernetes Event를 발생시킵니다.
`kubectl get events`로 클러스터 활동을 실시간 확인합니다.

```bash
# 특정 클러스터의 이벤트 감시
kubectl get events --field-selector involvedObject.name=my-cluster -w

# 네임스페이스의 모든 AerospikeCluster 이벤트 조회
kubectl get events --field-selector involvedObject.kind=AerospikeCluster -n aerospike
```

### 이벤트 레퍼런스

| Reason | Type | 설명 |
|--------|------|------|
| `RollingRestartStarted` | Normal | 롤링 리스타트 루프 시작; 랙 ID와 파드 수 표시 |
| `RollingRestartCompleted` | Normal | 모든 대상 파드에 대해 롤링 리스타트 완료 |
| `PodWarmRestarted` | Normal | 파드가 SIGUSR1 수신 (무중단 설정 리로드) |
| `PodColdRestarted` | Normal | 파드가 삭제 후 재생성됨 (풀 리스타트) |
| `RestartFailed` | Warning | 롤링 리스타트 중 파드 재시작 실패 |
| `LocalPVCDeleteFailed` | Warning | 콜드 리스타트 전 로컬 PVC 삭제 실패 |
| `ConfigMapCreated` | Normal | 랙 ConfigMap 최초 생성 |
| `ConfigMapUpdated` | Normal | 랙 ConfigMap 새 설정으로 업데이트 |
| `DynamicConfigApplied` | Normal | 재시작 없이 파드에 설정 변경 적용 |
| `DynamicConfigStatusFailed` | Warning | 동적 설정 상태 업데이트 실패 |
| `StatefulSetCreated` | Normal | 랙 StatefulSet 최초 생성 |
| `StatefulSetUpdated` | Normal | 랙 StatefulSet 스펙 업데이트 |
| `RackScaled` | Normal | 랙 레플리카 수 변경; 이전/새 수 표시 |
| `ACLSyncStarted` | Normal | ACL 역할/사용자 동기화 시작 |
| `ACLSyncCompleted` | Normal | ACL 역할과 사용자 동기화 성공 |
| `ACLSyncError` | Warning | ACL 동기화 중 오류 발생 |
| `PDBCreated` | Normal | PodDisruptionBudget 생성 |
| `PDBUpdated` | Normal | PodDisruptionBudget 업데이트 |
| `ServiceCreated` | Normal | Headless 서비스 생성 |
| `ServiceUpdated` | Normal | Headless 서비스 업데이트 |
| `ClusterDeletionStarted` | Normal | 클러스터 삭제 시작 (finalizer 활성) |
| `FinalizerRemoved` | Normal | 스토리지 finalizer 제거; 객체 삭제 예정 |
| `ReadinessGateSatisfied` | Normal | 파드 readiness gate `acko.io/aerospike-ready`가 True로 설정됨 |
| `ReadinessGateBlocking` | Warning | Readiness gate 대기 중 롤링 리스타트 차단됨 |
| `TemplateApplied` | Normal | ClusterTemplate 스펙이 이 클러스터에 적용됨 |
| `TemplateDrifted` | Warning | 클러스터 스펙이 템플릿에서 드리프트됨 |
| `TemplateResolutionError` | Warning | ClusterTemplate 해결 또는 적용 실패 |
| `ValidationWarning` | Warning | 비차단 검증 경고 감지 |
| `PVCCleanedUp` | Normal | 스케일 다운 후 고아 PVC 삭제 |
| `PVCCleanupFailed` | Warning | 스케일 다운 후 고아 PVC 삭제 실패 |
| `CircuitBreakerActive` | Warning | 연속 실패 후 재조정 백오프 |
| `CircuitBreakerReset` | Normal | 성공적인 재조정 후 서킷 브레이커 리셋 |
| `ReconcileError` | Warning | 재조정 루프에서 복구 불가능한 오류 발생 |
| `Operation` | Normal | 온디맨드 오퍼레이션 이벤트 |
