---
sidebar_position: 3
title: 접근 제어 (ACL)
---

# 접근 제어 (ACL)

오퍼레이터의 접근 제어 스펙으로 Aerospike CE 인증과 권한을 설정합니다.

## 개요

Aerospike CE 접근 제어(ACL)는 접속할 수 있는 사용자와 각 사용자가 수행할 수 있는 작업을 제한합니다. ACL을 활성화하면 다음 규칙이 적용됩니다.

- 모든 클라이언트 연결은 사용자 이름과 비밀번호로 인증해야 합니다.
- 각 사용자에게 권한을 정의하는 **role**을 하나 이상 부여합니다.
- 오퍼레이터가 Aerospike 관리 API로 사용자와 role을 생성하고 관리합니다.

:::warning
Aerospike CE 8.x는 `aerospike.conf`의 `security` stanza를 지원하지 않습니다. ACL은 오퍼레이터의 `aerospikeAccessControl` 스펙에서만 설정하세요. 오퍼레이터는 이 스펙을 바탕으로 런타임에 Aerospike 관리 API를 호출해 사용자와 role을 적용합니다.
:::

## 사전 준비

### Kubernetes Secret 생성

각 사용자의 비밀번호는 Kubernetes Secret에 저장해야 합니다. Secret에는 `password` 키가 포함되어야 합니다.

```bash
# admin 사용자 Secret 생성
kubectl -n aerospike create secret generic admin-secret \
  --from-literal=password='admin-password-here'

# 애플리케이션 사용자 Secret 생성
kubectl -n aerospike create secret generic app-secret \
  --from-literal=password='app-password-here'
```

## 기본 ACL 설정

최소 ACL 구성은 `sys-admin`과 `user-admin` 역할을 모두 가진 사용자가 최소 한 명 필요합니다.

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-acl
  namespace: aerospike
spec:
  size: 3
  image: aerospike:ce-8.1.1.1
  aerospikeAccessControl:
    users:
      - name: admin
        secretName: admin-secret
        roles:
          - sys-admin
          - user-admin
      - name: appuser
        secretName: app-secret
        roles:
          - read-write
  aerospikeConfig:
    service:
      cluster-name: aerospike-acl
    namespaces:
      - name: test
        replication-factor: 2
        storage-engine:
          type: memory
```

## 내장 역할

Aerospike CE는 아래 role을 미리 정의합니다. `roles` 목록에 다시 정의하지 않고 사용자에게 바로 부여할 수 있습니다.

| 역할 | 설명 |
|------|------|
| `user-admin` | 사용자 생성/삭제, 역할 부여/회수 |
| `sys-admin` | 클러스터 관리 (truncate, config, info 명령) |
| `data-admin` | 인덱스 관리 (보조 인덱스 생성/삭제, UDF) |
| `read` | 레코드 읽기 |
| `write` | 레코드 쓰기 (삽입/수정/삭제) |
| `read-write` | 레코드 읽기 및 쓰기 |
| `read-write-udf` | 레코드 읽기, 쓰기, UDF 실행 |
| `truncate` | 네임스페이스/셋 Truncate |

:::info
`superuser` 역할은 Aerospike Enterprise Edition에서만 존재합니다. CE 클러스터는 위의 내장 역할을 사용하거나 커스텀 역할을 정의해야 합니다.
:::

## 커스텀 역할

세밀한 권한으로 커스텀 역할을 정의할 수 있습니다.

```yaml
spec:
  aerospikeAccessControl:
    roles:
      - name: inventory-reader
        privileges:
          - read.inventory           # 'inventory' 네임스페이스 읽기
      - name: orders-writer
        privileges:
          - read-write.orders        # 'orders' 네임스페이스 읽기/쓰기
          - read-write.orders.items  # 'orders.items' 셋 읽기/쓰기
    users:
      - name: admin
        secretName: admin-secret
        roles:
          - sys-admin
          - user-admin
      - name: inventory-svc
        secretName: inventory-secret
        roles:
          - inventory-reader
      - name: orders-svc
        secretName: orders-secret
        roles:
          - orders-writer
```

### 권한 형식

권한은 `<코드>[.<네임스페이스>[.<셋>]]` 형식을 따릅니다.

| 코드 | 설명 |
|------|------|
| `read` | 레코드 읽기 |
| `write` | 레코드 쓰기 |
| `read-write` | 레코드 읽기 및 쓰기 |
| `read-write-udf` | 레코드 읽기, 쓰기, UDF 실행 |
| `sys-admin` | 시스템 관리 |
| `user-admin` | 사용자 관리 |
| `data-admin` | 데이터 관리 |
| `truncate` | 데이터 Truncate |

**예시:**
- `read` — 모든 네임스페이스에 대한 전역 읽기
- `read-write.myns` — `myns` 네임스페이스에 대한 읽기/쓰기
- `write.myns.myset` — `myns` 내 `myset` 셋에 대한 쓰기

## 역할 화이트리스트 (IP 허용 목록)

각 커스텀 역할에 `whitelist` 필드를 포함하여 해당 역할로 인증할 수 있는 클라이언트 IP 주소를 제한할 수 있습니다. 화이트리스트 CIDR은 Aerospike 서버 레벨에서 적용되어, Kubernetes NetworkPolicy 외에 추가적인 네트워크 기반 접근 제어 계층을 제공합니다.

```yaml
spec:
  aerospikeAccessControl:
    roles:
      - name: internal-reader
        privileges:
          - read.data
        whitelist:
          - "10.0.0.0/8"         # 내부 네트워크만 허용
          - "172.16.0.0/12"
      - name: monitoring-role
        privileges:
          - read
        whitelist:
          - "10.100.0.0/16"      # 모니터링 서브넷만 허용
    users:
      - name: admin
        secretName: admin-secret
        roles:
          - sys-admin
          - user-admin
      - name: internal-app
        secretName: internal-app-secret
        roles:
          - internal-reader
      - name: prometheus
        secretName: prometheus-secret
        roles:
          - monitoring-role
```

**화이트리스트 사용 시기:**

- 데이터베이스 접근을 특정 애플리케이션 서브넷으로 제한
- 모니터링 접근을 Prometheus/Grafana 네트워크 범위로 제한
- Kubernetes NetworkPolicy와 함께 심층 방어 구현

:::note
화이트리스트는 역할별로 적용됩니다. 사용자가 여러 역할을 가진 경우, 할당된 역할 중 하나라도 허용하는 IP에서 접속할 수 있습니다. 내장 역할(예: `read-write`)은 화이트리스트를 지원하지 않으므로, 이 기능을 사용하려면 커스텀 역할을 생성해야 합니다.
:::

## Admin 정책

`adminPolicy` 필드는 오퍼레이터가 수행하는 관리 API 작업(사용자/역할 생성, 비밀번호 변경 등)의 타임아웃을 설정합니다. Aerospike 클러스터가 높은 부하 상태에 있고 관리 작업이 기본 2초 타임아웃보다 오래 걸릴 수 있을 때 유용합니다.

```yaml
spec:
  aerospikeAccessControl:
    adminPolicy:
      timeout: 5000    # 5초 (기본값: 2000ms)
    users:
      - name: admin
        secretName: admin-secret
        roles:
          - sys-admin
          - user-admin
```

| 필드 | 기본값 | 범위 | 설명 |
|------|--------|------|------|
| `timeout` | `2000` | 100 -- 30000 ms | 관리 작업 타임아웃 (밀리초) |

특히 다수의 사용자/역할이 있는 초기 클러스터 생성 시 또는 높은 부하 상태에서 타임아웃 관련 오류 메시지와 함께 `ACLSyncError` 이벤트가 발생하면 이 값을 증가시키세요.

## 비밀번호 교체

오퍼레이터는 `aerospikeAccessControl.users[*].secretName`에서 참조하는 Kubernetes Secret을 감시합니다. Secret의 데이터가 변경되면 오퍼레이터는 AerospikeCluster CR을 변경할 필요 없이 업데이트된 비밀번호를 Aerospike에 자동으로 동기화합니다.

```bash
# Secret을 업데이트하여 사용자 비밀번호 교체
kubectl -n aerospike create secret generic app-secret \
  --from-literal=password='new-password-here' \
  --dry-run=client -o yaml | kubectl apply -f -
```

이벤트를 통해 동기화를 확인합니다:

```bash
kubectl get events --field-selector reason=ACLSyncStarted -n aerospike
kubectl get events --field-selector reason=ACLSyncCompleted -n aerospike
```

:::info
AerospikeCluster의 ACL 설정에서 활발히 참조하는 Secret만 재조정을 트리거합니다. 동일한 네임스페이스의 관련 없는 Secret 변경은 무시됩니다.
:::

## Webhook 검증 규칙

오퍼레이터의 admission webhook은 다음 ACL 규칙을 적용합니다.

1. **Admin 사용자 필수**: 최소 한 명의 사용자가 `sys-admin`과 `user-admin` 역할을 모두 보유해야 합니다. 이 조건이 충족되지 않으면 초기 설정 이후 오퍼레이터가 ACL을 관리할 수 없습니다.

2. **역할 교차 검증**: 사용자가 참조하는 모든 역할은 내장 역할이거나 `roles` 목록에 명시적으로 정의되어 있어야 합니다. 정의되지 않은 역할 참조는 거부됩니다.

3. **권한 코드 검증**: 커스텀 역할의 각 권한은 유효한 권한 코드를 사용해야 합니다. 유효하지 않은 코드는 설명적인 오류와 함께 거부됩니다.

4. **Secret 필수**: 모든 사용자는 비밀번호가 포함된 Kubernetes Secret을 가리키는 `secretName`이 있어야 합니다.

## 문제 해결

### 일반적인 검증 오류

**"must have at least one user with both 'sys-admin' and 'user-admin' roles"**

최소 한 명의 사용자가 두 역할을 모두 보유하고 있는지 확인하세요.

```yaml
users:
  - name: admin
    secretName: admin-secret
    roles:
      - sys-admin
      - user-admin    # 동일한 사용자에게 두 역할 모두 필요
```

**"user X references undefined role Y"**

해당 역할이 내장 역할이거나 `roles` 목록에 정의되어 있어야 합니다.

```yaml
roles:
  - name: custom-role       # 커스텀 역할 정의
    privileges:
      - read.myns
users:
  - name: myuser
    secretName: myuser-secret
    roles:
      - custom-role          # 이제 이 참조가 유효합니다
```

**"role X has invalid privilege code Y"**

유효한 권한 코드(`read`, `write`, `read-write`, `read-write-udf`, `sys-admin`, `user-admin`, `data-admin`, `truncate`)만 사용하세요.

```yaml
roles:
  - name: my-role
    privileges:
      - read-write.myns     # 유효
      # - admin.myns        # 유효하지 않음 — 'sys-admin' 또는 'data-admin' 사용
```

### ACL 상태 확인

배포 후 ACL이 정상 동작하는지 확인합니다.

```bash
# ACL 동기화에 대한 오퍼레이터 로그 확인
kubectl -n aerospike-operator logs -l control-plane=controller-manager | grep -i acl

# 인증 정보로 클러스터에 접속
kubectl -n aerospike exec -it aerospike-acl-0-0 -- asadm -Uadmin -Padmin-password-here

# 사용자 목록 확인 (asadm 내부)
manage acl show users
```
