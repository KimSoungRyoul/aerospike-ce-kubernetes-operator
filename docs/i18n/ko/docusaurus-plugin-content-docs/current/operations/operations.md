---
sidebar_position: 2
title: 오퍼레이션
---

# 오퍼레이션

`spec.operations`에 파드 재시작을 선언하면 오퍼레이터가 실행 순서를 제어하고 상태와 Kubernetes 이벤트를 기록합니다.

---

## 개요

Aerospike 파드를 직접 삭제하지 말고 on-demand operation을 사용하세요. 클러스터 스펙에 오퍼레이션을 추가하면 오퍼레이터가 순서, 상태 추적, 이벤트 발행을 처리합니다.

**주요 제약사항:**

- 한 번에 **하나의 오퍼레이션**만 활성화할 수 있습니다.
- 오퍼레이션 `id`는 고유해야 하며 길이는 1--20자여야 합니다.
- `InProgress` 상태에서는 오퍼레이션을 수정할 수 없습니다.
- 완료되면 스펙에서 오퍼레이션을 제거하세요.

---

## 오퍼레이션 유형

### WarmRestart

WarmRestart는 Aerospike 프로세스에 `SIGUSR1` 신호를 보내 파드 삭제 없이 설정을 리로드합니다. 파드는 프로세스 전체에서 계속 실행됩니다.

```yaml
spec:
  operations:
    - kind: WarmRestart
      id: "config-reload-v2"
```

**사용 시기:**

- 프로세스 신호가 필요한 동적 설정 변경 후
- 다운타임을 최소화하고 싶을 때 (파드 삭제/재생성 없음)
- Aerospike가 SIGUSR1을 통해 지원하는 설정 리로드에 사용

:::tip
WarmRestart는 가장 덜 파괴적인 오퍼레이션입니다. Aerospike 프로세스가 제자리에서 재시작되어 파드의 네트워크 ID와 스토리지 마운트를 유지합니다.
:::

### PodRestart

PodRestart(콜드 재시작)는 대상 파드를 삭제하고 재생성합니다. 볼륨 재연결을 포함한 전체 재시작 사이클과 동일합니다.

```yaml
spec:
  operations:
    - kind: PodRestart
      id: "cold-restart-01"
```

**사용 시기:**

- WarmRestart로 충분하지 않을 때 (예: 파드가 비정상 상태)
- 볼륨 재초기화를 강제하고 싶을 때
- Aerospike 프로세스가 SIGUSR1에 응답하지 않을 때
- 새로운 파드 스케줄링이 필요한 노드 유지보수 후

---

## 특정 파드 대상 지정

기본적으로 오퍼레이션은 클러스터의 **모든 파드**를 대상으로 합니다. `podList`를 사용하여 특정 파드를 대상으로 지정할 수 있습니다:

### 모든 파드 (기본값)

```yaml
spec:
  operations:
    - kind: WarmRestart
      id: "reload-all"
      # podList 생략 = 모든 파드
```

### 특정 파드

```yaml
spec:
  operations:
    - kind: PodRestart
      id: "restart-pod-2"
      podList:
        - aerospike-3node-0
        - aerospike-3node-2
```

:::warning
특정 파드를 대상으로 지정할 때 올바른 파드 이름을 사용하세요. 파드 이름은 멀티 랙 배포의 경우 `<클러스터명>-<랙ID>-<순서>`, 단일 랙 클러스터의 경우 `<클러스터명>-<순서>` 패턴을 따릅니다.
:::

---

## 오퍼레이션 트리거

### 1단계: 클러스터 스펙에 오퍼레이션 추가

```bash
kubectl -n aerospike patch asc aerospike-3node --type merge -p '{
  "spec": {
    "operations": [
      {
        "kind": "WarmRestart",
        "id": "config-reload-v2"
      }
    ]
  }
}'
```

### 2단계: 진행 상황 모니터링

```bash
# 오퍼레이션 상태 확인
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='{.status.operationStatus}' | jq .

# 이벤트 감시
kubectl -n aerospike get events \
  --field-selector involvedObject.name=aerospike-3node,reason=Operation -w
```

### 3단계: 완료 후 오퍼레이션 제거

오퍼레이션이 `Completed` 또는 `Error` 단계에 도달하면 스펙에서 제거하세요:

```bash
kubectl -n aerospike patch asc aerospike-3node --type merge -p '{
  "spec": {
    "operations": null
  }
}'
```

---

## 오퍼레이션 상태 추적

오퍼레이터는 `status.operationStatus`에서 오퍼레이션 진행 상황을 추적합니다:

```bash
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='{.status.operationStatus}' | jq .
```

### 상태 필드

| 필드 | 타입 | 설명 |
|------|------|------|
| `id` | string | 오퍼레이션 식별자 (`spec.operations[].id`와 일치) |
| `kind` | string | 오퍼레이션 유형: `WarmRestart` 또는 `PodRestart` |
| `phase` | string | 현재 단계: `InProgress`, `Completed` 또는 `Error` |
| `completedPods` | []string | 오퍼레이션이 완료된 파드 목록 |
| `failedPods` | []string | 오퍼레이션이 실패한 파드 목록 |

### 단계 전환

```
                    ┌──────────────┐
                    │  InProgress  │
                    └──────┬───────┘
                           │
              ┌────────────┴────────────┐
              ▼                         ▼
     ┌────────────────┐       ┌─────────────┐
     │   Completed    │       │    Error     │
     └────────────────┘       └─────────────┘
```

| 단계 | 의미 |
|------|------|
| `InProgress` | 오퍼레이터가 대상 파드에서 오퍼레이션을 실행 중 |
| `Completed` | 모든 대상 파드가 성공적으로 재시작됨 |
| `Error` | 하나 이상의 파드가 실패. 자세한 내용은 `failedPods` 확인 |

### 상태 출력 예시

```json
{
  "id": "config-reload-v2",
  "kind": "WarmRestart",
  "phase": "InProgress",
  "completedPods": [
    "aerospike-3node-0",
    "aerospike-3node-1"
  ],
  "failedPods": []
}
```

---

## 오퍼레이션 후 파드 상태

오퍼레이션이 완료된 후 개별 파드 상태에 재시작 정보가 반영됩니다:

```bash
kubectl -n aerospike get asc aerospike-3node \
  -o jsonpath='{.status.pods}' | jq 'to_entries[] | {pod: .key, lastRestart: .value.lastRestartReason, time: .value.lastRestartTime}'
```

`lastRestartReason` 필드는 파드가 마지막으로 재시작된 이유를 기록합니다:

| 값 | 설명 |
|----|------|
| `WarmRestart` | 온디맨드 웜 재시작 (SIGUSR1) |
| `ManualRestart` | 온디맨드 콜드 재시작 (PodRestart) |
| `ConfigChanged` | 설정 변경으로 인한 콜드 재시작 |
| `ImageChanged` | 이미지 업데이트로 인한 콜드 재시작 |
| `PodSpecChanged` | 파드 스펙 변경으로 인한 콜드 재시작 |

---

## WarmRestart vs PodRestart

| 측면 | WarmRestart | PodRestart |
|------|-------------|------------|
| **메커니즘** | Aerospike 프로세스에 SIGUSR1 신호 | 파드 삭제 및 재생성 |
| **다운타임** | 최소 (프로세스 제자리 재시작) | 전체 파드 라이프사이클 (종료, 스케줄링, 시작) |
| **스토리지** | 볼륨이 마운트된 상태 유지 | 볼륨 분리 후 재연결 |
| **네트워크** | 파드 IP 유지 | 새로운 IP를 받을 수 있음 |
| **사용 사례** | 설정 리로드, 정상 재시작 | 멈춘 파드, 볼륨 리셋, 노드 마이그레이션 |
| **위험도** | 낮음 -- 프로세스가 깨끗하게 재시작 | 중간 -- 파드 재스케줄링, 마이그레이션 가능성 |

:::info
클러스터의 `rollingUpdateBatchSize`는 온디맨드 오퍼레이션에 적용되지 **않습니다**. 오퍼레이션은 오퍼레이터의 내부 순서에 따라 모든 대상 파드에서 실행됩니다.
:::

---

## 검증 규칙

웹훅은 오퍼레이션에 대해 다음 제약조건을 적용합니다:

| 규칙 | 제약조건 | 오류 메시지 |
|------|----------|------------|
| 최대 오퍼레이션 수 | 한 번에 1개만 | `only one operation allowed` |
| ID 길이 | 1--20자 | `id must be between 1 and 20 characters` |
| 진행 중 잠금 | `InProgress` 중 수정 불가 | `cannot modify operations while InProgress` |

---

## 이벤트

오퍼레이터는 오퍼레이션 라이프사이클에 대한 이벤트를 발행합니다:

| Reason | Type | 설명 |
|--------|------|------|
| `Operation` | Normal | 오퍼레이션 시작, 파드 재시작 또는 오퍼레이션 완료 |
| `PodWarmRestarted` | Normal | 파드가 SIGUSR1을 수신 |
| `PodColdRestarted` | Normal | 파드가 삭제되고 재생성됨 |

```bash
kubectl -n aerospike get events \
  --field-selector involvedObject.name=aerospike-3node,reason=Operation
```

---

## 예시

### 모든 파드에 설정 리로드

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-3node
  namespace: aerospike
spec:
  size: 3
  image: aerospike:ce-8.1.1.1
  operations:
    - kind: WarmRestart
      id: "reload-config-mar10"
  aerospikeConfig:
    namespaces:
      - name: test
        replication-factor: 2
        storage-engine:
          type: memory
```

### 단일 파드 콜드 재시작

```yaml
spec:
  operations:
    - kind: PodRestart
      id: "fix-pod-2"
      podList:
        - aerospike-3node-2
```

### 파드 부분 집합 웜 재시작

```yaml
spec:
  operations:
    - kind: WarmRestart
      id: "reload-rack-1"
      podList:
        - aerospike-3node-0
        - aerospike-3node-1
```

---

## 모범 사례

1. **설명적인 오퍼레이션 ID 사용** -- 날짜나 버전 참조를 포함하세요 (예: `config-reload-mar10`, `fix-pod-2-v3`). 상태 추적과 이벤트 상관관계 분석이 쉬워집니다.
2. **가능하면 WarmRestart 선호** -- 파괴를 최소화하고 파드 재스케줄링을 피합니다.
3. **완료된 오퍼레이션 제거** -- 오래된 오퍼레이션을 스펙에 남겨두어도 문제가 발생하지 않지만, 스펙을 깨끗하게 유지하면 혼동을 피할 수 있습니다.
4. **새 오퍼레이션 추가 전 상태 확인** -- 웹훅은 `InProgress` 상태에서 두 번째 오퍼레이션을 거부합니다.
5. **대상 지정된 재시작에 `podList` 사용** -- 하나의 파드에만 주의가 필요할 때 전체 클러스터를 재시작하지 마세요.
