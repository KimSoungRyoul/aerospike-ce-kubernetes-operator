---
sidebar_position: 10
title: 트러블슈팅
---

# 트러블슈팅

이 가이드는 Aerospike CE Kubernetes Operator의 일반적인 문제, 디버깅 기법, 진단에 필요한 참조 정보를 다룹니다.

## 증상별 진단

| 증상 | 확인 명령 | 가능한 원인 | 해결 방법 |
|------|----------|------------|----------|
| Phase = `Error` | `kubectl get asc <name> -o jsonpath='{.status.lastReconcileError}'` | 잘못된 설정, 이미지 풀 실패, 리소스 부족 | 에러 메시지 기반으로 수정 후 재적용 |
| Phase = `WaitingForMigration` | `kubectl exec <pod> -- asinfo -v 'statistics' \| grep migrate` | 데이터 마이그레이션 진행 중 | 완료 대기 (자동 재시도) |
| `InProgress`에서 멈춤 | `kubectl get pvc -n <ns> -l aerospike.io/cr-name=<name>` | PVC Pending, 이미지 풀 실패, 스케줄링 실패 | StorageClass/이미지/리소스 확인 |
| `CircuitBreakerActive` 이벤트 | `kubectl get asc <name> -o jsonpath='{.status.failedReconcileCount}'` | 10회 이상 연속 실패 | `lastReconcileError` 확인 후 근본 원인 수정 |
| Pod `CrashLoopBackOff` | `kubectl logs <pod> -c aerospike-server --previous` | 설정 파싱 오류, 메모리 부족 | 서버 로그 확인 후 설정 수정 |
| Webhook이 CR 거부 | `kubectl apply` 에러 메시지 확인 | CE 제약 위반 | [검증 에러 패턴](#검증-에러-패턴) 참조 |
| `dynamicConfigStatus=Failed` | `kubectl get asc <name> -o jsonpath='{.status.pods}' \| jq '.[].dynamicConfigStatus'` | 동적 변경 불가 파라미터 | `enableDynamicConfigUpdate: false`로 변경하여 롤링 재시작 유도 |
| `ReadinessGateBlocking` | `kubectl get pod <pod> -o jsonpath='{.status.conditions}' \| jq '.[]'` | Readiness gate 미충족 | 파드 Aerospike 상태 및 마이그레이션 상태 점검 |

## 일반적인 문제와 해결 방법

### 클러스터가 InProgress에서 멈춤

클러스터 phase가 `InProgress`에 머물러 `Completed`로 전환되지 않는 경우입니다.

**가능한 원인:**

1. **PVC 바인딩 실패** — StorageClass가 존재하지 않거나 용량이 부족합니다.
2. **이미지 풀 실패** — 클러스터에서 컨테이너 이미지에 접근할 수 없습니다.
3. **스케줄링 실패** — 파드의 스케줄링 제약을 만족하는 노드가 없습니다.
4. **서킷 브레이커 활성화** — 반복 실패 후 오퍼레이터가 백오프 상태입니다.

**진단 방법:**

```bash
# 클러스터 phase 및 이유 확인
kubectl -n aerospike get asc <name> -o jsonpath='{.status.phase}{"\t"}{.status.phaseReason}'

# Pending 상태의 PVC 확인
kubectl -n aerospike get pvc -l aerospike.io/cr-name=<name>

# 파드 이벤트에서 스케줄링/풀 에러 확인
kubectl -n aerospike describe pod <pod-name>

# 오퍼레이터 로그 확인
kubectl -n aerospike-operator logs -l control-plane=controller-manager | tail -50

# 서킷 브레이커 상태 확인
kubectl -n aerospike get asc <name> \
  -o jsonpath='{.status.failedReconcileCount}{"\t"}{.status.lastReconcileError}'
```

### 파드 미준비 상태 (CrashLoopBackOff)

Aerospike 파드가 시작되지만 반복적으로 크래시합니다.

**가능한 원인:**

1. **잘못된 aerospikeConfig** — 네임스페이스 이름, 스토리지 경로, 파라미터가 올바르지 않습니다.
2. **메모리 부족** — 설정된 네임스페이스에 비해 컨테이너 메모리 제한이 너무 낮습니다.
3. **스토리지 경로 불일치** — 설정된 파일 경로가 마운트된 볼륨과 일치하지 않습니다.

**진단 방법:**

```bash
# 현재 크래시 로그 확인
kubectl -n aerospike logs <pod-name> -c aerospike-server

# 이전 크래시 로그 확인
kubectl -n aerospike logs <pod-name> -c aerospike-server --previous

# 파드 상태 상세 정보
kubectl -n aerospike describe pod <pod-name>
```

### 스케일링 실패

스케일 업 또는 스케일 다운이 완료되지 않는 경우입니다.

**스케일 업 문제:**
- `spec.size`가 8을 초과하지 않는지 확인합니다 (CE 제한).
- `replication-factor`가 새 클러스터 크기를 초과하지 않는지 확인합니다.
- 충분한 클러스터 리소스(CPU, 메모리, 스토리지)가 있는지 확인합니다.

**스케일 다운 문제:**
- 오퍼레이터는 파드를 제거하기 전에 데이터 마이그레이션이 완료될 때까지 대기합니다.
- `ScaleDownDeferred` 이벤트가 마이그레이션으로 인해 스케일 다운이 차단되었음을 나타내는지 확인합니다.

```bash
# 스케일 다운 지연 이벤트 확인
kubectl get events --field-selector reason=ScaleDownDeferred -n aerospike

# 마이그레이션 상태 확인
kubectl -n aerospike exec <pod-name> -c aerospike-server -- asinfo -v 'statistics' | grep migrate_partitions_remaining
```

### ACL 동기화 실패

ACL 역할이나 사용자가 Aerospike 클러스터와 동기화되지 않는 경우입니다.

**가능한 원인:**

1. **Secret 누락** — 참조된 Kubernetes Secret이 존재하지 않습니다.
2. **password 키 누락** — Secret에 `password` 키가 포함되어 있지 않습니다.
3. **관리자 사용자 없음** — `sys-admin`과 `user-admin` 역할을 모두 가진 사용자가 없습니다.

**진단 방법:**

```bash
# ACL 동기화 이벤트 확인
kubectl get events --field-selector reason=ACLSyncError -n aerospike

# Secret 존재 여부 및 password 키 확인
kubectl -n aerospike get secret <secret-name> -o jsonpath='{.data.password}' | base64 -d

# 오퍼레이터 로그에서 ACL 에러 확인
kubectl -n aerospike-operator logs -l control-plane=controller-manager | grep -i acl
```

### 동적 설정 변경 실패

설정 변경이 런타임에 적용되지 않고 재시작을 트리거하는 경우입니다.

**가능한 원인:**

1. **`enableDynamicConfigUpdate` 미설정** — 동적 업데이트는 기본적으로 비활성화되어 있습니다.
2. **정적 파라미터 변경** — `replication-factor`, `storage-engine type`, `name` 등은 항상 재시작이 필요합니다.
3. **유효하지 않은 문자** — `;` 또는 `:`가 포함된 파라미터 값은 사전 검증에서 거부됩니다.
4. **부분 실패 후 롤백** — 배치 내 하나의 변경이 실패하면 오퍼레이터가 적용된 모든 변경을 롤백하고 콜드 재시작으로 전환합니다.

**진단 방법:**

```bash
# 파드별 동적 설정 상태 확인
kubectl -n aerospike get asc <name> -o jsonpath='{.status.pods}' | \
  jq '.[] | {name: .podName, dynamicConfig: .dynamicConfigStatus}'

# 동적 설정 이벤트 확인
kubectl get events --field-selector reason=DynamicConfigApplied -n aerospike
kubectl get events --field-selector reason=DynamicConfigStatusFailed -n aerospike

# 오퍼레이터 로그에서 롤백 확인
kubectl -n aerospike-operator logs -l control-plane=controller-manager | grep -i "rollback\|dynamic config"
```

| 상태 | 의미 |
|------|------|
| `Applied` | 동적 설정이 런타임에 성공적으로 적용됨 |
| `Failed` | 동적 업데이트 실패 — 롤링 재시작이 트리거됨 |
| `Pending` | 오퍼레이터가 변경을 적용하기를 대기 중 |
| (비어있음) | 동적 설정 변경이 시도되지 않음 |

## 서킷 브레이커와 복구

오퍼레이터에는 지속적으로 실패하는 클러스터에 대한 과도한 재시도를 방지하기 위한 서킷 브레이커가 내장되어 있습니다.

### 동작 방식

**10회 연속 조정(reconciliation) 실패** 후, 오퍼레이터는 지수 백오프 지연과 함께 백오프 상태에 진입합니다:

| 연속 실패 횟수 | 백오프 지연 |
|--------------|-----------|
| 1 | 2초 |
| 2 | 4초 |
| 3 | 8초 |
| 5 | 32초 |
| 8회 이상 | 약 4.3분 (256초 상한) |

서킷 브레이커가 활성화된 동안 실패 횟수와 마지막 에러를 포함한 `CircuitBreakerActive` 경고 이벤트가 발생합니다.

:::info
**영구적 검증 에러**(예: 잘못된 Aerospike 설정 구조, ACL 시크릿 누락, 잘못된 권한 코드)는 `failedReconcileCount`를 최대 임계값으로 즉시 설정하여 서킷 브레이커를 **즉시 활성화**합니다. `PermanentError` 이벤트가 발생하고, `ReconcileHealthy` 상태 조건이 `False` (reason: `PermanentError`)로 설정됩니다. 이러한 에러는 자동으로 복구되지 않으므로 spec을 수정해야 합니다.
:::

### 서킷 브레이커 상태 확인

```bash
# 활성화된 서킷 브레이커 이벤트 확인
kubectl get events --field-selector reason=CircuitBreakerActive -n aerospike

# 실패 횟수 및 마지막 에러 확인
kubectl -n aerospike get asc <name> \
  -o jsonpath='{.status.failedReconcileCount}{"\t"}{.status.lastReconcileError}'
```

### 서킷 브레이커 리셋

서킷 브레이커는 성공적인 조정 후 자동으로 리셋됩니다. 복구를 위해서는:

1. **근본 원인 수정** — `lastReconcileError`를 확인하고 근본적인 문제를 해결합니다.
2. **수정된 spec 재적용** — `kubectl apply -f <fixed-cr.yaml>`
3. **리셋 확인** — `CircuitBreakerReset` 이벤트를 확인합니다.

```bash
kubectl get events --field-selector reason=CircuitBreakerReset -n aerospike
```

## 디버깅 명령

### 클러스터 상태

```bash
# 모든 클러스터 목록 및 phase
kubectl get asc -n <ns>

# 특정 클러스터 phase 및 이유
kubectl get asc <name> -o jsonpath='{.status.phase}'
kubectl get asc <name> -o jsonpath='{.status.phaseReason}'

# 컨디션 확인
kubectl get asc <name> -o jsonpath='{.status.conditions}' | jq .

# 서킷 브레이커 상태
kubectl get asc <name> -o jsonpath='{.status.failedReconcileCount}'
kubectl get asc <name> -o jsonpath='{.status.lastReconcileError}'
```

### 파드 상태

```bash
# 클러스터의 모든 파드 상태
kubectl get asc <name> -o jsonpath='{.status.pods}' | jq .

# Ready 파드 수
kubectl get asc <name> -o jsonpath='{.status.size}'

# 재시작 대기 중인 파드
kubectl get asc <name> -o jsonpath='{.status.pendingRestartPods}'

# 템플릿 동기화 상태
kubectl get asc <name> -o jsonpath='{.status.templateSnapshot.synced}'
```

### 이벤트

```bash
# 특정 클러스터의 이벤트 (시간순 정렬)
kubectl get events -n <ns> --field-selector involvedObject.name=<name> --sort-by='.lastTimestamp'

# 실시간 이벤트 감시
kubectl get events -n <ns> -w

# 특정 이벤트 이유로 필터링
kubectl get events --field-selector reason=CircuitBreakerActive -n <ns>
kubectl get events --field-selector reason=ACLSyncError -n <ns>
kubectl get events --field-selector reason=RestartFailed -n <ns>
```

### 로그

```bash
# 오퍼레이터 로그
kubectl -n aerospike-operator logs -l control-plane=controller-manager -f

# Aerospike 서버 로그 (현재)
kubectl -n <ns> logs <pod> -c aerospike-server -f

# Aerospike 서버 로그 (이전 크래시)
kubectl -n <ns> logs <pod> -c aerospike-server --previous
```

## 검증 에러 패턴

Webhook은 AerospikeCluster를 생성하거나 업데이트할 때 CE 제약을 검증합니다. 아래는 일반적인 검증 에러와 해결 방법입니다.

### 크기 및 이미지 에러

| 에러 메시지 | 원인 | 해결 방법 |
|------------|------|----------|
| `spec.size N exceeds CE maximum of 8` | 클러스터 크기가 CE 제한 초과 | `spec.size`를 8 이하로 설정 |
| `spec.image must not be empty` | 이미지 미지정, templateRef도 없음 | `spec.image`에 유효한 CE 이미지 설정 |
| `spec.image "..." is an Enterprise Edition image` | EE 이미지 태그 사용 | Community Edition 이미지 사용 (예: `aerospike:ce-8.1.1.1`) |

### Aerospike 설정 에러

| 에러 메시지 | 원인 | 해결 방법 |
|------------|------|----------|
| `must not contain 'xdr' section` | XDR은 Enterprise 전용 | `aerospikeConfig`에서 `xdr` 섹션 제거 |
| `must not contain 'tls' section` | TLS는 Enterprise 전용 | `aerospikeConfig`에서 `tls` 섹션 제거 |
| `namespaces count N exceeds CE maximum of 2` | 2개 초과 네임스페이스 | 2개 이하로 축소 |
| `heartbeat.mode must be 'mesh'` | 비mesh heartbeat 모드 | `network.heartbeat.mode`를 `mesh`로 설정 |

### Enterprise 전용 Namespace 키

다음 키들은 CE 네임스페이스 설정에서 허용되지 않습니다:

`compression`, `compression-level`, `durable-delete`, `fast-restart`, `index-type`, `sindex-type`, `rack-id`, `strong-consistency`, `tomb-raider-eligible-age`, `tomb-raider-period`

에러 형식: `namespace[N] "name": 'key' is not allowed (reason)`

### ACL 검증 에러

| 에러 메시지 | 원인 | 해결 방법 |
|------------|------|----------|
| `must have at least one user with both 'sys-admin' and 'user-admin' roles` | 관리자 사용자 미정의 | 최소 한 명의 사용자에게 두 역할 할당 |
| `user "name" must have a secretName for password` | 비밀번호 Secret 참조 누락 | 사용자 spec에 `secretName` 추가 |
| `duplicate user name "name"` | 중복 사용자 이름 | 각 사용자에 고유한 이름 사용 |
| `user "name" references undefined role "role"` | 정의되지 않은 커스텀 역할 참조 | `aerospikeAccessControl.roles`에 역할 추가 또는 내장 역할 사용 |

유효한 권한 코드: `read`, `write`, `read-write`, `read-write-udf`, `sys-admin`, `user-admin`, `data-admin`, `truncate`

권한 형식: `"<code>"` / `"<code>.<namespace>"` / `"<code>.<namespace>.<set>"`

### Rack 설정 검증 에러

| 에러 메시지 | 원인 | 해결 방법 |
|------------|------|----------|
| `rack ID must be > 0` | Rack ID가 0 또는 음수 | 1부터 시작하는 rack ID 사용 |
| `duplicate rack ID N` | 여러 rack에서 같은 ID 사용 | 고유한 rack ID 사용 |
| `duplicate rackLabel "label"` | 여러 rack에서 같은 레이블 사용 | 고유한 rack 레이블 사용 |
| `rackConfig rack IDs cannot be changed` | 업데이트 시 rack ID 변경 시도 | Rack ID는 생성 후 변경 불가 |

### 스토리지 검증 에러

| 에러 메시지 | 원인 | 해결 방법 |
|------------|------|----------|
| `duplicate volume name "name"` | 같은 볼륨 이름 중복 사용 | 고유한 볼륨 이름 사용 |
| `exactly one volume source must be specified` | 볼륨 소스가 0개 또는 2개 이상 | 정확히 하나의 소스 지정 (persistentVolume, emptyDir 등) |
| `persistentVolume.size must not be empty` | PV 크기 미지정 | 유효한 크기 설정 (예: `10Gi`) |
| `aerospike.path must be an absolute path` | 볼륨 마운트에 상대 경로 사용 | 절대 경로 사용 (예: `/opt/aerospike/data`) |
| `subPath and subPathExpr are mutually exclusive` | 같은 마운트에 둘 다 설정 | `subPath` 또는 `subPathExpr` 중 하나만 사용 |

### 네임스페이스 검증 에러

| 에러 메시지 | 원인 | 해결 방법 |
|------------|------|----------|
| `replication-factor must be between 1 and 4` | RF 범위 초과 | 1에서 4 사이의 값으로 설정 |
| `replication-factor N exceeds cluster size M` | RF가 노드 수보다 큼 | RF를 낮추거나 `spec.size` 증가 |

## 스토리지 관련 문제

### PVC 바인딩 실패

PersistentVolumeClaim이 `Pending` 상태에 머무르는 경우입니다.

```bash
# PVC 상태 확인
kubectl -n aerospike get pvc -l aerospike.io/cr-name=<name>

# PVC 이벤트 상세 확인
kubectl -n aerospike describe pvc <pvc-name>

# StorageClass 존재 여부 확인
kubectl get sc
```

**일반적인 원인:**
- StorageClass가 존재하지 않거나 잘못 설정되어 있습니다.
- 사용 가능한 PersistentVolume이 없습니다 (정적 프로비저닝의 경우).
- 프로비저너의 스토리지 용량이 부족합니다.
- 볼륨 토폴로지 제약으로 인해 스케줄된 노드에서 바인딩할 수 없습니다.

### Cascade Delete 동작

볼륨(또는 글로벌 볼륨 정책)에 `cascadeDelete: true`가 설정된 경우, 다음 상황에서 PVC가 자동으로 삭제됩니다:
- AerospikeCluster CR이 삭제될 때.
- 파드가 스케일 다운될 때 (파드가 완전히 종료된 후).

**스케일 다운 시 PVC 정리:**
- 오퍼레이터는 모든 스케일 다운 파드가 종료될 때까지 기다린 후 PVC를 삭제합니다.
- 파드가 `Terminating` 상태에서 멈추면 PVC 정리는 다음 조정 주기로 연기됩니다.
- `PVCCleanedUp` 및 `PVCCleanupFailed` 이벤트에서 상태를 확인합니다.

```bash
# Terminating 상태에서 멈춘 파드 확인
kubectl -n aerospike get pods | grep Terminating

# PVC 정리 이벤트 확인
kubectl get events --field-selector reason=PVCCleanedUp -n aerospike
kubectl get events --field-selector reason=PVCCleanupFailed -n aerospike
```

:::warning
`cascadeDelete: true`가 설정되지 않은 PVC는 CR 삭제 후에도 항상 보존됩니다. 더 이상 필요하지 않은 경우 수동으로 삭제해야 합니다.
:::

### 로컬 스토리지 문제

로컬 스토리지 클래스를 `deleteLocalStorageOnRestart: true`와 함께 사용하는 경우:
- 콜드 재시작 시 파드 삭제 전에 로컬 스토리지 클래스 기반 PVC가 삭제됩니다.
- 이로 인해 새 노드에서 재프로비저닝이 강제됩니다.
- `deleteLocalStorageOnRestart`가 설정되지 않으면 로컬 PVC가 유지되어 파드가 다른 노드로 이동할 경우 스케줄링이 차단될 수 있습니다.

```bash
# 로컬 PVC 삭제 실패 확인
kubectl get events --field-selector reason=LocalPVCDeleteFailed -n aerospike
```

## 네트워크 관련 문제

### 파드 연결

파드 간 연결이 안 되거나 클라이언트가 클러스터에 접근할 수 없는 경우입니다.

```bash
# 파드 IP 및 준비 상태 확인
kubectl -n aerospike get pods -o wide

# 헤드리스 서비스 확인
kubectl -n aerospike get svc

# Aerospike 클러스터 메시 상태 확인
kubectl -n aerospike exec <pod-name> -c aerospike-server -- asinfo -v 'statistics' | grep cluster_size

# 클러스터 상태에서 네트워크 엔드포인트 확인
kubectl -n aerospike get asc <name> -o jsonpath='{.status.pods}' | \
  jq '.[] | {pod: .podName, ip: .podIP, endpoints: .accessEndpoints}'
```

### Mesh Heartbeat 문제

CE 오퍼레이터는 `heartbeat.mode`가 `mesh`여야 합니다. 노드가 클러스터를 구성할 수 없는 경우:

1. **mesh 모드 확인** — `aerospikeConfig.network.heartbeat.mode`가 `mesh`로 설정되어 있는지 확인합니다.
2. **mesh 주소 확인** — 오퍼레이터는 헤드리스 서비스를 통해 mesh 시드 주소를 자동 설정합니다.
3. **DNS 해석** — 파드가 헤드리스 서비스 DNS 이름을 해석할 수 있는지 확인합니다.

```bash
# 파드 내에서 DNS 해석 확인
kubectl -n aerospike exec <pod-name> -c aerospike-server -- nslookup <headless-svc-name>

# Aerospike 네트워크 정보 확인
kubectl -n aerospike exec <pod-name> -c aerospike-server -- asinfo -v 'mesh'
```

### 호스트 네트워크 문제

`hostNetwork: true` 사용 시:
- 포트 충돌을 방지하기 위해 `multiPodPerHost`는 `false`여야 합니다.
- 올바른 DNS 해석을 위해 `dnsPolicy`는 `ClusterFirstWithHostNet`이어야 합니다.
- 오퍼레이터가 이 기본값을 자동으로 설정하지만, 불일치 시 검증 경고가 발생합니다.

## 이벤트 참조

오퍼레이터는 주요 생명주기 전환 시 Kubernetes Event를 발생시킵니다. 이 이벤트를 통해 클러스터 활동을 모니터링할 수 있습니다.

### 롤링 재시작 이벤트

| Reason | 타입 | 설명 |
|--------|------|------|
| `RollingRestartStarted` | Normal | 롤링 재시작 루프 시작 |
| `RollingRestartCompleted` | Normal | 모든 대상 파드 재시작 완료 |
| `PodWarmRestarted` | Normal | 설정 리로드를 위한 SIGUSR1 전송 |
| `PodColdRestarted` | Normal | 파드 삭제 후 재생성 |
| `RestartFailed` | Warning | 파드 재시작 실패 |

### 설정 이벤트

| Reason | 타입 | 설명 |
|--------|------|------|
| `ConfigMapCreated` | Normal | 랙 ConfigMap 생성 |
| `ConfigMapUpdated` | Normal | ConfigMap 내용 변경 |
| `DynamicConfigApplied` | Normal | 런타임 설정 변경 적용 |
| `DynamicConfigStatusFailed` | Warning | 동적 설정 변경 실패 |

### StatefulSet 및 Rack 이벤트

| Reason | 타입 | 설명 |
|--------|------|------|
| `StatefulSetCreated` | Normal | 랙 StatefulSet 생성 |
| `StatefulSetUpdated` | Normal | StatefulSet 스펙 업데이트 |
| `RackScaled` | Normal | 랙 파드 수 변경 |
| `ScaleDownDeferred` | Warning | 데이터 마이그레이션으로 스케일 다운 차단 |

### ACL 이벤트

| Reason | 타입 | 설명 |
|--------|------|------|
| `ACLSyncStarted` | Normal | ACL 동기화 시작 |
| `ACLSyncCompleted` | Normal | ACL 동기화 성공 |
| `ACLSyncError` | Warning | ACL 동기화 에러 발생 |

### 스토리지 이벤트

| Reason | 타입 | 설명 |
|--------|------|------|
| `PVCCleanedUp` | Normal | 스케일 다운 후 고아 PVC 삭제 |
| `PVCCleanupFailed` | Warning | 고아 PVC 삭제 실패 |
| `LocalPVCDeleteFailed` | Warning | 콜드 재시작 전 로컬 PVC 삭제 실패 |

### 템플릿 이벤트

| Reason | 타입 | 설명 |
|--------|------|------|
| `TemplateApplied` | Normal | ClusterTemplate 스펙 적용 |
| `TemplateDrifted` | Warning | 클러스터 스펙이 템플릿과 불일치 |
| `TemplateResolutionError` | Warning | ClusterTemplate 해석 실패 |

### 인프라 이벤트

| Reason | 타입 | 설명 |
|--------|------|------|
| `PDBCreated` | Normal | PodDisruptionBudget 생성 |
| `PDBUpdated` | Normal | PodDisruptionBudget 업데이트 |
| `ServiceCreated` | Normal | 헤드리스 서비스 생성 |
| `ServiceUpdated` | Normal | 헤드리스 서비스 업데이트 |

### 생명주기 이벤트

| Reason | 타입 | 설명 |
|--------|------|------|
| `ClusterDeletionStarted` | Normal | 클러스터 삭제 시작 |
| `FinalizerRemoved` | Normal | Finalizer 제거, 오브젝트 삭제 예정 |
| `ReadinessGateSatisfied` | Normal | 파드 readiness gate 충족 |
| `ReadinessGateBlocking` | Warning | Readiness gate로 롤링 재시작 차단 |

### 서킷 브레이커 이벤트

| Reason | 타입 | 설명 |
|--------|------|------|
| `CircuitBreakerActive` | Warning | 연속 실패 후 조정 백오프 |
| `CircuitBreakerReset` | Normal | 성공 후 서킷 브레이커 리셋 |

### 기타 이벤트

| Reason | 타입 | 설명 |
|--------|------|------|
| `ValidationWarning` | Warning | 비차단 검증 경고 |
| `ReconcileError` | Warning | 조정 중 에러 발생 |
| `Operation` | Normal | 온디맨드 오퍼레이션 처리 |

### Quiesce 이벤트

| Reason | 타입 | 설명 |
|--------|------|------|
| `NodeQuiesceStarted` | Normal | 노드 quiesce 시작 |
| `NodeQuiesced` | Normal | 노드 quiesce 완료 |
| `NodeQuiesceFailed` | Warning | 노드 quiesce 실패 |
