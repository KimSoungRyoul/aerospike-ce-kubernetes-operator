---
sidebar_position: 6
title: 클러스터 매니저 UI
---

# Aerospike 클러스터 매니저 UI

[Aerospike Cluster Manager](https://github.com/aerospike-ce-ecosystem/aerospike-cluster-manager)는 Aerospike CE 클러스터를 관리하기 위한 웹 기반 GUI입니다. 오퍼레이터 Helm 차트에 번들로 포함되어 있으며, 오퍼레이터와 함께 선택적 컴포넌트로 배포할 수 있습니다.

UI에는 클러스터 연결 프로파일을 저장하기 위한 PostgreSQL 사이드카(PVC 포함)가 내장되어 있습니다.

---

## 설치

오퍼레이터 설치 시 UI를 활성화합니다.

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --namespace aerospike-operator --create-namespace \
  --set ui.enabled=true
```

:::note RBAC 권한
`ui.rbac.create=true`(기본값)일 때, Helm 차트가 생성하는 ClusterRole에는 `autoscaling` API 그룹의 `horizontalpodautoscalers` 리소스에 대한 전체 접근 권한이 포함됩니다. 이 권한은 UI에서 HPA를 생성, 조회, 삭제하는 데 필요합니다.
:::

UI Pod가 실행 중인지 확인합니다.

```bash
kubectl -n aerospike-operator get pods -l app.kubernetes.io/component=ui
```

---

## UI 접속

### 포트 포워딩 (개발용)

```bash
kubectl -n aerospike-operator port-forward svc/acko-aerospike-ce-kubernetes-operator-ui 3000:3000
```

브라우저에서 [http://localhost:3000](http://localhost:3000)을 엽니다.

:::tip
서비스 이름은 `<릴리스명>-aerospike-ce-kubernetes-operator-ui` 패턴을 따릅니다. 다른 릴리스 이름을 사용한 경우 그에 맞게 조정하세요.
```bash
kubectl -n aerospike-operator port-forward svc/<릴리스명>-aerospike-ce-kubernetes-operator-ui 3000:3000
```
:::

### Ingress (운영 환경)

외부에서 지속적으로 접근하려면 Ingress를 활성화합니다.

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --namespace aerospike-operator --create-namespace \
  --set ui.enabled=true \
  --set ui.ingress.enabled=true \
  --set ui.ingress.className=nginx \
  --set "ui.ingress.hosts[0].host=aerospike-admin.example.com" \
  --set "ui.ingress.hosts[0].paths[0].path=/" \
  --set "ui.ingress.hosts[0].paths[0].pathType=Prefix"
```

---

## 설정 옵션

| 파라미터 | 설명 | 기본값 |
|----------|------|--------|
| `ui.enabled` | 클러스터 매니저 UI 활성화 | `false` |
| `ui.replicaCount` | UI 레플리카 수 | `1` |
| `ui.image.repository` | UI 컨테이너 이미지 | `ghcr.io/aerospike-ce-ecosystem/aerospike-cluster-manager` |
| `ui.image.tag` | 이미지 태그 (비어 있으면 Chart appVersion 사용) | `""` |
| `ui.service.type` | 서비스 타입 (`ClusterIP`, `NodePort`, `LoadBalancer`) | `ClusterIP` |
| `ui.service.frontendPort` | 프론트엔드 (Next.js) 포트 | `3000` |
| `ui.service.backendPort` | 백엔드 (FastAPI) 포트 | `8000` |
| `ui.postgresql.enabled` | 내장 PostgreSQL 사이드카 배포 | `true` |
| `ui.k8s.enabled` | K8s 클러스터 관리 기능 활성화 | `true` |
| `ui.ingress.enabled` | 외부 접근용 Ingress 생성 | `false` |
| `ui.persistence.enabled` | PostgreSQL 데이터용 PVC 활성화 | `true` |
| `ui.persistence.size` | PVC 스토리지 크기 | `1Gi` |
| `ui.env.databaseUrl` | 외부 PostgreSQL URL (`postgresql.enabled=false` 일 때) | `""` |
| `ui.rbac.create` | K8s API 접근용 ClusterRole 및 ClusterRoleBinding 생성 (AerospikeCluster, Template, HPA 관리 권한 포함) | `true` |
| `ui.serviceAccount.create` | UI Pod용 ServiceAccount 생성 | `true` |
| `ui.networkPolicy.enabled` | UI Pod 네트워크 트래픽 제한 | `false` |
| `ui.image.pullPolicy` | 이미지 풀 정책 | `IfNotPresent` |
| `ui.service.annotations` | 서비스 어노테이션 (클라우드 LB 설정 등) | `{}` |
| `ui.resources.requests.cpu` | UI 컨테이너 CPU 요청 | `100m` |
| `ui.resources.requests.memory` | UI 컨테이너 메모리 요청 | `256Mi` |
| `ui.resources.limits.cpu` | UI 컨테이너 CPU 제한 | `200m` |
| `ui.resources.limits.memory` | UI 컨테이너 메모리 제한 | `512Mi` |
| `ui.persistence.storageClassName` | PostgreSQL PVC 스토리지 클래스 | `""` (기본값) |
| `ui.postgresql.existingSecret` | 데이터베이스 자격 증명에 기존 Secret 사용 | `""` |
| `ui.extraEnv` | UI 컨테이너에 추가할 환경 변수 목록 | `[]` |

:::tip
프로브, 보안 컨텍스트, 톨러레이션, 어피니티, 오토스케일링 등 전체 설정 옵션 목록은 다음 명령으로 확인할 수 있습니다.
```bash
helm show values oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator | grep -A 500 "^ui:"
```
:::

---

## 기능

### 연결 관리

색상 코드가 있는 프로파일로 여러 Aerospike 클러스터 연결을 관리합니다. 각 연결은 호스트, 포트, 선택적 인증 정보를 저장하며, 프로파일은 내장 PostgreSQL 데이터베이스에 영속 저장됩니다.

### 클러스터 모니터링

TPS, 클라이언트 연결 수, 성공률을 보여주는 실시간 대시보드입니다. 네임스페이스 사용량, 스토리지 활용도, 클러스터 상태를 한눈에 확인할 수 있습니다.

### 레코드 브라우저

페이지네이션을 지원하여 레코드를 탐색, 생성, 수정, 삭제합니다. 네임스페이스와 셋을 탐색하고, 전체 메타데이터와 함께 개별 레코드를 검사할 수 있습니다.

### 쿼리 빌더

프레디케이트를 사용하여 스캔/쿼리 작업을 빌드하고 실행합니다. AQL을 직접 작성하지 않고 시각적으로 쿼리를 구성합니다.

### HPA (Horizontal Pod Autoscaler) 관리

클러스터 상세 페이지에서 AerospikeCluster 리소스를 대상으로 하는 HorizontalPodAutoscaler(HPA)를 관리할 수 있습니다. HPA는 CPU 또는 메모리 사용량에 따라 클러스터 크기를 자동으로 조정합니다.

**HPA 생성**: 클러스터 상세 페이지의 작업 메뉴에서 **HPA 관리**를 선택하여 새 HPA를 생성합니다. 다음 항목을 설정할 수 있습니다:

- **최소 레플리카(Min Replicas)** — 자동 스케일링 시 유지할 최소 Pod 수
- **최대 레플리카(Max Replicas)** — 허용할 최대 Pod 수
- **CPU 목표 사용률(CPU Target Utilization)** — 스케일 아웃을 트리거하는 평균 CPU 사용률(%)
- **메모리 목표 사용률(Memory Target Utilization)** — 스케일 아웃을 트리거하는 평균 메모리 사용률(%)

생성된 HPA는 해당 AerospikeCluster를 `scaleTargetRef`로 참조합니다.

**HPA 조회**: 클러스터에 연결된 HPA가 존재하면 현재 레플리카 수, 목표 메트릭, 현재 메트릭 값을 확인할 수 있습니다.

**HPA 삭제**: 더 이상 자동 스케일링이 필요하지 않은 경우 UI에서 HPA를 삭제할 수 있습니다. 삭제 후에는 수동 스케일링으로 전환됩니다.

:::note
HPA 관리 기능을 사용하려면 UI의 ClusterRole에 `autoscaling` API 그룹에 대한 권한이 필요합니다. `ui.rbac.create=true`(기본값)일 때 자동으로 설정됩니다.
:::

### K8s 클러스터 관리

`ui.k8s.enabled=true`(기본값)인 경우 UI는 Kubernetes 네이티브 클러스터 관리 기능을 제공합니다.

- **클러스터 생성** — 안내형 마법사로 새 AerospikeCluster CR 배포
- **클러스터 편집** — 편집 다이얼로그를 통해 실행 중인 클러스터 설정 수정 (이미지, 크기, 동적 설정, Aerospike 설정)
- **클러스터 스케일** — 클러스터 크기 조정 (CE는 1~8 노드)
- **상태 모니터링** — 클러스터 단계, 조건, Pod 상세 정보 조회
- **템플릿 관리** — AerospikeClusterTemplate 생성, 탐색, 삭제 및 동기화 상태 확인
- **템플릿 CRUD** — 전체 템플릿 라이프사이클: 기본 이미지, 크기, 리소스, 스케줄링, 스토리지, 모니터링, 네트워크 설정으로 템플릿 생성; RBAC 수정으로 편집 다이얼로그를 통한 템플릿 패치/업데이트 지원; `usedBy` 사용 현황 추적이 포함된 템플릿 상세 보기; 미사용 템플릿 삭제 (클러스터가 먼저 링크 해제해야 함)
- **템플릿 스케줄링 설정** — `podAntiAffinityLevel`, `tolerations`, `nodeAffinity`, `topologySpreadConstraints`를 포함한 전체 스케줄링 제약 조건 구성
- **템플릿 재동기화** — 클러스터 상세 페이지의 **Template Resync** 버튼으로 최신 템플릿 설정을 클러스터에 다시 적용 (`acko.io/resync-template=true` 어노테이션 트리거)
- **템플릿 스냅샷** — 동기화 상태(동기화됨/비동기화)와 함께 해결된 템플릿 스펙 조회
- **작업 트리거** — 선택적 Pod 지정을 통한 웜 재시작 및 Pod 재시작 개시
- **Pod 선택** — 체크박스로 특정 Pod를 선택하여 타겟 재시작 작업 수행
- **일시 정지/재개** — 조정(Reconciliation) 일시 정지 및 재개
- **ACL 설정** — 역할, 사용자, K8s Secret 기반 인증 정보로 접근 제어 설정 (Secret 선택 드롭다운 포함)
- **롤링 업데이트 전략** — 배치 크기, 최대 불가용 수, PDB 설정 구성
- **작업 모니터링** — 활성 작업 상태, 완료/실패한 Pod 실시간 조회
- **동적 설정 상태** — Pod별 동적 설정 상태(Applied/Failed/Pending) 및 마지막 재시작 사유 조회
- **조정 추적** — 조정 오류 횟수 및 실패 원인 확인
- **이벤트 조회** — 각 클러스터의 K8s 이벤트 타임라인 탐색 (전환 단계에서 자동 갱신)
- **Pod 로그** — Pod 테이블에서 직접 컨테이너 로그 조회 (tail lines 선택, 복사, 다운로드)
- **CR YAML 내보내기** — 디버깅 또는 마이그레이션을 위해 클러스터의 AerospikeCluster CR을 깔끔한 YAML로 복사
- **클러스터 복제** — 기존 클러스터의 spec을 복사하여 새 이름/네임스페이스로 클러스터를 생성 (operations와 paused 상태 제외). 다른 네임스페이스로 복제 시 ACL Secret은 자동 복사되지 않으므로, 대상 네임스페이스에 동일 Secret을 미리 생성해야 합니다.
- **헬스 대시보드** — 클러스터 상태 한눈에 보기: Pod 준비 상태, 마이그레이션 상태, 설정 상태, 가용성, 랙 분배
- **스토리지 정책** — PVC의 볼륨 초기화 방식(deleteFiles/dd/blkdiscard/headerCleanup), 와이프 방식, 캐스케이드 삭제 동작 설정
- **네트워크 접근 타입** — 클라이언트가 클러스터에 접근하는 방식 선택: Pod IP (기본값), 호스트 내부 IP, 호스트 외부 IP, 또는 설정된 IP; 노드 간 통신용 Fabric 타입 설정
- **노드 차단 목록** — Aerospike Pod가 스케줄되어서는 안 되는 Kubernetes 노드 지정 (마법사 + 편집 다이얼로그)
- **대역폭 제한** — CNI 대역폭 어노테이션으로 인그레스/이그레스 트래픽 제한 설정 (마법사 + 편집 다이얼로그)
- **HPA 관리** — HorizontalPodAutoscaler 리소스의 생성, 조회, 삭제를 통한 CPU/메모리 기반 자동 스케일링
- **모니터링 고급 설정** — exporter 이미지, 메트릭 라벨, exporter 환경 변수, exporter 리소스(CPU/메모리), ServiceMonitor(enabled/interval/labels), PrometheusRule(enabled/labels/customRules) 구성. `customRules` 지정 시 기본 알림(NodeDown, StopWrites, HighDiskUsage, HighMemoryUsage)을 사용자 정의 규칙으로 대체
- **Seeds Finder 고급 설정** — LoadBalancer 서비스의 어노테이션, 라벨, source ranges 구성
- **서비스 메타데이터** — headless 서비스와 per-pod 서비스에 커스텀 annotations/labels 추가 (External DNS 통합, 서비스 메시 연동 등)
- **Pod 메타데이터** — Aerospike Pod에 커스텀 labels/annotations 추가 (Istio 사이드카 주입, 모니터링 셀렉터 등)
- **랙별 오버라이드** — per-rack storage(다른 StorageClass/크기), per-rack tolerations/affinity/nodeSelector 설정
- **랙 Revision** — 각 랙에 revision 문자열을 설정하여 랙 단위 롤링 리스타트 트리거
- **컨테이너 보안 컨텍스트** — 컨테이너 수준 보안 설정 (runAsUser, runAsGroup, privileged, readOnlyRootFilesystem, allowPrivilegeEscalation, runAsNonRoot)
- **Priority Class Name** — Pod 스케줄링 섹션에서 K8s PriorityClass 설정
- **작업 초기화** — Operation Status 카드의 **Clear** 버튼으로 멈춘 operation 해제 (`spec.operations` 초기화)
- **클라이언트 사이드 검증** — CE 8 미만 이미지 거부, Enterprise 에디션 이미지 거부, `xdr`/`tls` 금지 키 차단, 랙 동시 추가+삭제 방지, replication factor vs size 교차 검증, CE 네임스페이스 제한 (최대 2개), 진행 중인 operation 트리거 차단
- **Operation In-Progress Guard** — **Warm Restart**와 **Pod Restart** 버튼이 다른 Operation 진행 중일 때 자동 비활성화되어 이중 작업 방지
- **Split-brain 감지** — `asinfo`가 보고하는 클러스터 크기와 K8s Pod 수가 다를 때 경고 배너 표시
- **서킷 브레이커 리셋** — Reconciliation Health 카드의 전용 **Reset** 버튼으로 원클릭 서킷 브레이커 리셋 (이전에는 수동 CR 패치 필요)
- **고아 PVC 삭제** — PVC 상태 목록에서 활성 Pod와 연결되지 않은 PVC를 직접 삭제

:::info
K8s 클러스터 관리 기능은 UI 서비스 어카운트가 AerospikeCluster 리소스에 대한 RBAC 접근 권한을 가져야 합니다. `ui.rbac.create=true`(기본값)인 경우 자동으로 설정됩니다.
:::

### 마이그레이션 상태 표시

클러스터가 스케일링이나 설정 변경 등으로 파티션 마이그레이션이 진행 중일 때, Overview 페이지에서 실시간 마이그레이션 상태를 확인할 수 있습니다.

- **실시간 진행률** — 남은 파티션 수와 프로그레스 바로 마이그레이션 진행 상황을 표시합니다.
- **Pod별 마이그레이션 분석** — 각 Pod가 마이그레이션하고 있는 파티션 수를 Pod 테이블에서 확인할 수 있습니다.
- **자동 새로고침** — 마이그레이션이 활성화된 동안 5초 간격으로 자동 갱신됩니다.
- **시각적 표시** — 프로그레스 바와 상태 배지로 마이그레이션 상태를 직관적으로 표현합니다.
- **랙 토폴로지 통합** — 랙 토폴로지 뷰에서 마이그레이션 중인 Pod가 하이라이트됩니다.

마이그레이션이 완료되면 상태 표시가 자동으로 사라지며, 클러스터가 안정(Stable) 상태로 전환됩니다.

### 랙 설정

마법사에는 다중 랙, 존 인식 배포를 위한 **랙 설정** 단계가 포함되어 있습니다.

- **랙 추가/삭제**: 고유한 ID로 여러 랙 설정
- **존 어피니티**: 라이브 노드 데이터에서 K8s 가용 영역 선택
- **Revision**: 각 랙에 revision 문자열을 설정하여 랙 단위 롤링 리스타트를 트리거합니다. revision 값을 변경하면 해당 랙의 Pod만 순차적으로 재시작됩니다.
- **Pod 분배**: 각 랙의 노드당 최대 Pod 수 설정
- **분배 미리보기**: 랙 간 예상 Pod 분배 확인

각 랙은 별도의 StatefulSet을 생성하여 존 인식 고가용성을 지원합니다.

### 스토리지 정책

영속 스토리지(device 모드)를 사용할 때 마법사에서 다음을 설정할 수 있습니다.

- **초기화 방식**: 첫 사용 시 볼륨 준비 방법 (`none`, `deleteFiles`, `dd`, `blkdiscard`, `headerCleanup`)
- **와이프 방식**: Pod 재시작 시 더티 볼륨 정리 방법 (`none`, `deleteFiles`, `dd`, `blkdiscard`, `headerCleanup`, `blkdiscardWithHeaderCleanup`)
- **캐스케이드 삭제**: 클러스터 CR 삭제 시 PVC 자동 삭제 여부 (기본값: 활성화)

### 컨테이너 보안 컨텍스트

편집 다이얼로그의 **Security Context** 탭에서 Pod 수준 보안 컨텍스트 외에 **Container Security Context** 섹션을 통해 컨테이너 수준 보안을 설정할 수 있습니다. 다음 필드를 구성합니다:

| Field | Description |
|-------|-------------|
| **runAsUser** | 컨테이너 프로세스를 실행할 UID |
| **runAsGroup** | 컨테이너 프로세스의 기본 GID |
| **runAsNonRoot** | `true`로 설정하면 root(UID 0)로 실행되는 것을 방지합니다 |
| **privileged** | 특권 모드 활성화 여부 |
| **readOnlyRootFilesystem** | 루트 파일 시스템을 읽기 전용으로 마운트 |
| **allowPrivilegeEscalation** | 자식 프로세스의 권한 상승 허용 여부 |

이 설정은 `spec.podSpec.aerospikeContainer.securityContext`에 매핑됩니다.

:::note
Aerospike CE 공식 이미지는 root로 실행됩니다. `runAsNonRoot: true`를 설정하면 Pod가 시작되지 않을 수 있으므로 주의하세요.
:::

### 네트워크 접근

클라이언트-클러스터 간 및 노드 간 통신을 설정합니다.

- **클라이언트 접근 타입**: `pod` (기본값 — Pod IP 사용), `hostInternal` (노드 내부 IP), `hostExternal` (노드 외부 IP), `configuredIP` (어노테이션 기반)
- **Fabric 타입**: 노드 간 Fabric 통신용 네트워크 타입 (기본값: `pod`)

### 인덱스 관리

보조 인덱스를 생성, 조회, 삭제합니다. 인덱스 빌드 진행 상황을 모니터링하고 인덱스 통계를 확인합니다.

### 사용자/역할 관리 (ACL)

UI를 통해 Aerospike 사용자와 역할을 관리합니다. 커맨드라인 도구 없이 사용자 생성, 역할 부여, 비밀번호 변경이 가능합니다.

### UDF 관리

Aerospike 클러스터에 등록된 사용자 정의 함수(Lua 모듈)를 업로드, 조회, 삭제합니다.

### AQL 터미널

신택스 하이라이팅과 결과 포맷팅을 지원하는 AQL 명령을 브라우저에서 직접 실행합니다.

### 라이트/다크 테마

취향에 맞게 라이트 테마와 다크 테마 간 전환할 수 있습니다.

---

## 외부 PostgreSQL 사용

내장 사이드카 대신 기존 PostgreSQL 인스턴스를 사용하려면:

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --namespace aerospike-operator --create-namespace \
  --set ui.enabled=true \
  --set ui.postgresql.enabled=false \
  --set ui.env.databaseUrl="postgresql://user:pass@db-host:5432/aerospike_manager"
```

:::tip
`ui.postgresql.existingSecret`에 Secret 이름을 설정하여 기존 Kubernetes Secret의 데이터베이스 자격 증명을 사용할 수도 있습니다. Secret에는 `POSTGRES_PASSWORD`와 `DATABASE_URL` 키가 포함되어야 합니다.
:::

---

## 보안

### RBAC

`ui.rbac.create=true`(기본값)인 경우 Helm 차트는 UI 서비스 어카운트에 다음 권한을 부여하는 ClusterRole과 ClusterRoleBinding을 생성합니다.

- `AerospikeCluster` 리소스에 대한 **읽기/쓰기** 접근 (생성, 스케일, 수정, 삭제)
- `AerospikeClusterTemplate` 리소스에 대한 **생성/삭제/읽기** 접근 (전체 템플릿 관리)
- `HorizontalPodAutoscaler` 리소스에 대한 **전체** 접근 (`autoscaling` API 그룹 — HPA 생성, 조회, 수정, 삭제)
- Pod, Service, Event, Namespace에 대한 **읽기 전용** 접근 (클러스터 모니터링, 이벤트 타임라인, 마법사 드롭다운용)
- Pod 로그(`pods/log`)에 대한 **읽기 전용** 접근 (UI에서 컨테이너 로그 조회용)
- Secret에 대한 **목록 전용** 접근 (ACL 인증 정보 선택을 위한 이름 열거 — 내용은 읽지 않음)
- StorageClass에 대한 **목록 전용** 접근 (스토리지 마법사 드롭다운용)
- Node에 대한 **읽기 전용** 접근 (`get`, `list`) (랙 설정에 사용되는 가용 영역 정보 조회용)

### Pod 보안

UI는 기본적으로 비루트(`runAsUser: 1001`)로 실행되며, Next.js 런타임 요구 사항을 지원하기 위해 읽기 전용 루트 파일 시스템은 비활성화됩니다. 권한 상승이 차단되고 모든 Linux 기능이 제거됩니다.

### 네트워크 정책

UI Pod에 대한 트래픽을 제한합니다.

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --namespace aerospike-operator --create-namespace \
  --set ui.enabled=true \
  --set ui.networkPolicy.enabled=true
```

---

## 전체 예시

UI, 모니터링, Ingress를 모두 활성화하여 오퍼레이터를 배포합니다.

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --namespace aerospike-operator --create-namespace \
  --set ui.enabled=true \
  --set ui.ingress.enabled=true \
  --set ui.ingress.className=nginx \
  --set "ui.ingress.hosts[0].host=aerospike-admin.example.com" \
  --set "ui.ingress.hosts[0].paths[0].path=/" \
  --set "ui.ingress.hosts[0].paths[0].pathType=Prefix" \
  --set serviceMonitor.enabled=true \
  --set grafanaDashboard.enabled=true
```
