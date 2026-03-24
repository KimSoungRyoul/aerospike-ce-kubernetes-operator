---
sidebar_position: 10
title: Cluster Manager UI
---

# Aerospike Cluster Manager UI

[Aerospike Cluster Manager](https://github.com/aerospike-ce-ecosystem/aerospike-cluster-manager)는 Aerospike CE 클러스터를 관리하는 웹 기반 GUI입니다. Operator Helm 차트에 포함되어 선택적으로 배포할 수 있습니다.

---

## Installation

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --namespace aerospike-operator --create-namespace \
  --set ui.enabled=true
```

:::note RBAC permissions
`ui.rbac.create=true`(기본값)일 때, Helm 차트가 생성하는 ClusterRole에는 `autoscaling` API 그룹의 `horizontalpodautoscalers` 리소스에 대한 전체 접근 권한이 포함됩니다. 이 권한은 UI에서 HPA를 생성, 조회, 삭제하는 데 필요합니다.
:::

UI 파드 확인:

```bash
kubectl -n aerospike-operator get pods -l app.kubernetes.io/component=ui
```

포트 포워딩으로 접속:

```bash
kubectl -n aerospike-operator port-forward svc/acko-aerospike-ce-kubernetes-operator-ui 3000:3000
```

브라우저에서 `http://localhost:3000` 접속.

---

## Clusters

사이드바의 연결 목록에서 클러스터를 선택하거나, 메인 화면에서 카드로 확인합니다. 각 카드에는 연결 상태, 노드 수, 네임스페이스 수, Aerospike 버전이 표시됩니다.

![Clusters 홈 화면](/img/ui/clusters-home.png)

---

## Create Cluster

사이드바의 **Create Cluster** 또는 우상단 버튼으로 클러스터 생성 마법사를 시작합니다. 총 9단계로 구성됩니다:

**Step 1 — Basic**: 클러스터 이름, K8s 네임스페이스, 노드 수(1-8), Aerospike 이미지를 설정합니다.

![Create Cluster - Step 1 Basic](/img/ui/create-cluster-basic.png)

**Step 3 — Monitoring & Options**: 아래 항목을 설정합니다:

- **Prometheus Monitoring** — 메트릭 exporter sidecar 활성화 및 포트 설정. 추가 구성: exporter 이미지, 메트릭 라벨, exporter 리소스(CPU/메모리), ServiceMonitor(enabled/interval/labels), PrometheusRule(enabled/labels/customRules). PrometheusRule에서 `customRules`를 지정하면 기본 알림(NodeDown, StopWrites, HighDiskUsage, HighMemoryUsage)을 사용자 정의 규칙으로 완전히 대체합니다. 자세한 내용은 [Monitoring guide](monitoring.md)를 참조하세요.
- **Dynamic Config** — 재시작 없이 설정 변경 적용
- **Network Access** — 클라이언트 접근 방식(Pod IP, Host Internal/External, Configured IP). `configuredIP` 선택 시 custom network names 입력 필드가 표시됩니다.
- **Kubernetes NetworkPolicy** — K8s NetworkPolicy 자동 생성 (standard 또는 Cilium)
- **Seeds Finder LoadBalancer** — 외부 시드 검색용 LoadBalancer 서비스 생성. 아래 필드를 UI에서 설정할 수 있습니다:

| Field | Description |
|-------|-------------|
| **Port** | LoadBalancer 외부 포트 (기본값: 3000) |
| **Target Port** | 트래픽을 전달할 컨테이너 포트 (기본값: 3000) |
| **External Traffic Policy** | `Cluster` 또는 `Local`. `Local`로 설정하면 클라이언트 소스 IP가 보존됩니다. |
| **Annotations** | 클라우드 프로바이더별 LoadBalancer 설정 (예: AWS NLB 타입, internal scheme) |
| **Labels** | LoadBalancer 서비스에 추가할 커스텀 라벨 |
| **Source Ranges** | 트래픽을 허용할 CIDR 목록 (보안을 위해 특정 IP 대역만 허용) |

이 설정은 `spec.seedsFinderServices.loadBalancer` 필드에 매핑됩니다. 자세한 내용은 [Networking — SeedsFinderServices](networking.md#seedsfinderservices)를 참조하세요.

**Step 8 — Review**: 모든 설정을 최종 확인한 후 **Create Cluster** 버튼으로 배포합니다.

![Create Cluster - Step 8 Review](/img/ui/create-cluster-review.png)

---

## Cluster List

K8s Clusters 페이지에서 모든 AerospikeCluster를 카드 형태로 조회합니다. 각 카드에는 다음 정보가 표시됩니다:

- **Phase Badge** — 클러스터 상태 (Completed, InProgress, Error, ScalingUp 등)
- **Node Count** — 현재 클러스터 노드 수
- **Image** — 사용 중인 Aerospike 이미지
- **Age** — 클러스터 생성 후 경과 시간
- **Template Drift Badge** — 참조 중인 AerospikeClusterTemplate와 설정이 다를 때 경고 표시
- **Failed Reconcile Count Badge** — 연속 Reconciliation 실패 횟수 (stuck 클러스터 식별용)

---

## Edit Cluster

Edit 다이얼로그는 diff 기반 패치를 사용하여 변경된 필드만 적용합니다. 지원하는 편집 항목:

- **Image / Size** — 이미지 변경 및 스케일링
- **Resources** — CPU/Memory requests/limits 설정
- **ACL (Access Control)** — ACL 활성화/비활성화, 역할 관리(권한, CIDR Whitelist), 사용자 관리(K8s Secret 비밀번호)
- **Aerospike Config** — JSON 편집기로 Aerospike 설정 직접 수정
- **Dynamic Config** — 재시작 없는 설정 변경 활성화
- **Monitoring** — Prometheus exporter, ServiceMonitor, PrometheusRule 설정, Metric Labels, Exporter 환경 변수
- **Network** — Access Type, Fabric Type, NetworkPolicy 자동 생성, Bandwidth 제한
- **Storage** — 멀티 볼륨 관리 (PVC, EmptyDir, Secret, ConfigMap, HostPath)
- **Pod Scheduling** — NodeSelector, Tolerations, Affinity, Host Network, Service Account, Priority Class Name
- **Topology Spread** — Pod 분산 제약 조건
- **Security Context** — runAsUser, fsGroup, supplementalGroups
- **Sidecars / Init Containers** — 커스텀 컨테이너 추가
- **Service Metadata** — Pod Service, Headless Service 라벨/어노테이션
- **Seeds Finder Services** — LoadBalancer 시드 검색 서비스
- **Rack Topology** — 랙 추가/삭제, zone/region/nodeName/revision 할당, per-rack config/storage/scheduling 오버라이드
- **Aerospike Container Security Context** — Aerospike 컨테이너 전용 보안 설정 (runAsUser, runAsGroup, privileged, runAsNonRoot, readOnlyRootFilesystem, allowPrivilegeEscalation)
- **Node Block List** — K8s 노드 피커로 차단 노드 선택

**클라이언트 사이드 검증**: 편집 다이얼로그는 Operator webhook에 전송하기 전에 다음 검증을 수행합니다:
- CE 8 미만 이미지 거부, Enterprise 에디션 이미지 거부 (CE operator는 Enterprise 이미지를 지원하지 않음), `xdr`/`tls` 금지 키 차단, 랙 동시 추가+삭제 방지, replication factor vs size 교차 검증, CE 네임스페이스 제한 (최대 2개, 오퍼레이터 webhook 검증과 일치)

### Rack Topology Editing

Edit 다이얼로그의 **Rack Config** 탭에서 클러스터 생성 이후에도 랙 토폴로지를 변경할 수 있습니다. 지원하는 작업:

- **랙 추가** — 새로운 랙 ID와 함께 zone, region, nodeName을 지정하여 랙을 추가합니다.
- **랙 삭제** — 기존 랙을 제거합니다. 해당 랙의 Pod는 순차적으로 축소됩니다.
- **Zone / Region / NodeName 할당** — 각 랙의 토폴로지 제약 조건을 수정합니다. 드롭다운에서 기존 K8s 노드의 zone과 region 라벨 값을 선택하거나 직접 입력할 수 있습니다.
- **Revision** — 각 랙에 버전 식별자를 부여합니다. Revision을 변경하면 해당 랙의 모든 Pod가 순차적으로 재시작됩니다. 설정 변경 없이 특정 랙만 재시작하고 싶을 때 유용합니다.
- **Per-rack 오버라이드** — 랙별로 다음 항목을 클러스터 수준 설정과 다르게 구성할 수 있습니다:
  - **Aerospike Config** — 랙별 네임스페이스 메모리 크기, storage-engine 설정 등
  - **Storage** — 랙별 StorageClass, 볼륨 크기 변경 (예: 특정 AZ에서 io2 사용)
  - **Scheduling** — 랙별 nodeSelector, tolerations, affinity 오버라이드

:::warning
랙 토폴로지 변경은 StatefulSet 재구성을 수반하며, Pod 재시작이 발생할 수 있습니다. 프로덕션 환경에서는 변경 전에 Review 단계에서 영향 범위를 확인하세요.
:::

### Node Block List Picker

Edit 다이얼로그의 **Pod Scheduling** 탭에서 Node Block List를 설정할 때, K8s 클러스터의 실제 노드 목록이 체크박스 형태로 표시됩니다. 텍스트를 직접 입력하는 대신 노드를 시각적으로 선택하여 차단할 수 있습니다.

- **노드 목록 자동 조회** — K8s API에서 현재 클러스터의 노드를 가져와 표시합니다.
- **체크박스 선택** — 차단할 노드를 체크박스로 선택/해제합니다.
- **노드 정보 표시** — 각 노드의 이름, 상태(Ready/NotReady), zone, instance type 등 주요 라벨 정보가 함께 표시됩니다.
- **검색 필터** — 노드 수가 많은 경우 이름으로 필터링하여 빠르게 찾을 수 있습니다.

이 설정은 `spec.podSpec.nodeBlockList`에 매핑되며, 선택된 노드에서는 Aerospike Pod가 스케줄링되지 않습니다.

### Container Security Context

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

---

## Cluster Overview

클러스터를 선택하면 Overview 탭이 표시됩니다. 클러스터 Phase, Pod Ready 수, 헬스 조건(Stable / Config Applied / Available / ACL Synced), Pod 목록을 한눈에 확인합니다.

상단 버튼으로 **Scale**, **Edit**, **Warm Restart**, **Pod Restart**, **Pause**, **Delete** 작업을 실행할 수 있습니다.

**Operation In-Progress Guard**: **Warm Restart**와 **Pod Restart** 버튼은 다른 Operation이 진행 중일 때 자동으로 비활성화됩니다. 이는 실수로 인한 이중 작업을 방지하여 클러스터 안정성을 보호합니다.

**Split-brain Detection**: Aerospike `asinfo`가 보고하는 클러스터 크기와 예상 K8s Pod 수가 다를 경우, 클러스터 상세 페이지에 경고 배너가 표시됩니다. 이는 모든 노드가 서로를 인식하지 못하는 네트워크 파티션 상황을 나타낼 수 있습니다.

Operation이 `InProgress` 상태에서 멈출 경우, Operation Status 카드의 **Clear** 버튼으로 `spec.operations`를 초기화하여 클러스터를 언블록할 수 있습니다.

![Cluster Overview](/img/ui/cluster-overview.png)

**ACKO INFO** 탭에서는 Aerospike 노드 단위 상세 정보(Build, Edition, Uptime, Connections, Cluster Size)를 확인합니다.

![Cluster ACKO INFO](/img/ui/cluster-acko-info.png)

### Migration Status Display

클러스터가 스케일링이나 설정 변경 등으로 파티션 마이그레이션이 진행 중일 때, Overview 페이지에서 실시간 마이그레이션 상태를 확인할 수 있습니다.

- **실시간 진행률** — 남은 파티션 수와 프로그레스 바로 마이그레이션 진행 상황을 표시합니다.
- **Pod별 마이그레이션 분석** — 각 Pod가 마이그레이션하고 있는 파티션 수를 Pod 테이블에서 확인할 수 있습니다.
- **자동 새로고침** — 마이그레이션이 활성화된 동안 5초 간격으로 자동 갱신됩니다.
- **시각적 표시** — 프로그레스 바와 상태 배지로 마이그레이션 상태를 직관적으로 표현합니다.
- **랙 토폴로지 통합** — 랙 토폴로지 뷰에서 마이그레이션 중인 Pod가 하이라이트됩니다.

마이그레이션이 완료되면 상태 표시가 자동으로 사라지며, 클러스터가 안정(Stable) 상태로 전환됩니다.

### Disconnected State

Aerospike 연결이 끊어진 경우 Overview 및 Browser 페이지에서 스켈레톤 로딩 대신 연결 해제 상태 화면이 표시됩니다. `WifiOff` 아이콘과 함께 재연결을 안내하는 메시지가 나타납니다.

### Events Timeline

클러스터 상세 페이지의 **Events** 탭에서 Kubernetes 이벤트를 확인합니다. 각 이벤트에는 타입, 이유, 메시지, 발생 횟수, 그리고 상대적 시간(예: "2m ago")이 표시됩니다. Transitional Phase에서는 자동으로 새로고침됩니다.

### Event Category Filtering

이벤트 타임라인에서 카테고리별 필터링이 가능합니다. 11개 카테고리로 자동 분류됩니다:

| Category | Description | Example Events |
|----------|-------------|----------------|
| **Lifecycle** | 클러스터 생성/삭제 | ClusterCreated, ClusterDeletionStarted |
| **Rolling Restart** | 롤링 리스타트 | RollingRestartStarted/Completed, PodRestarted |
| **Configuration** | 설정 변경 | ConfigMapCreated, DynamicConfigApplied |
| **ACL Security** | 접근 제어 | ACLSyncStarted/Completed/Failed |
| **Scaling** | 스케일 업/다운 | RackScaled, PVCCleanupCompleted |
| **Rack Management** | 랙 관리 | StatefulSetCreated, RackRemoved |
| **Network** | 네트워크 리소스 | ServiceCreated, PDBCreated, NetworkPolicyCreated |
| **Monitoring** | 모니터링 설정 | MonitoringConfigured |
| **Template** | 템플릿 동기화 | TemplateApplied, TemplateOutOfSync |
| **Circuit Breaker** | 서킷 브레이커 | CircuitBreakerActive/Reset |

카테고리 필터 칩을 클릭하여 특정 유형의 이벤트만 표시할 수 있습니다.

### Event Export

이벤트 타임라인 헤더의 **JSON** 또는 **CSV** 다운로드 버튼을 클릭하여 이벤트를 내보낼 수 있습니다. 내보내기에는 이벤트 타입, 이유, 카테고리, 발생 횟수, 타임스탬프, 메시지가 포함됩니다. 카테고리 필터가 활성화된 경우 필터링된 이벤트만 내보내집니다.

### Configuration Drift Detection

클러스터 상세 페이지에서 **Config Status** 카드가 현재 설정의 동기화 상태를 표시합니다:

- **In Sync** — 원하는 설정(spec)과 적용된 설정(appliedSpec)이 일치
- **Config Drift Detected** — spec과 appliedSpec 사이에 차이 발견

변경된 필드 목록과 Pod별 설정 해시 버전이 표시됩니다. 여러 해시 그룹이 있으면 일부 Pod가 아직 이전 설정으로 실행 중임을 의미합니다.

Config Drift API는 `desiredConfig`과 `appliedConfig` 필드를 통해 실제 설정 값을 반환하므로, UI에서 상세한 diff 비교가 가능합니다. 두 가지 보기 모드를 지원합니다:

- **Fields 뷰** (기본) — 변경된 필드별로 원하는 값(`+`)과 적용된 값(`-`)을 색상 코딩(`added`, `removed`, `changed`)으로 표시합니다.
- **Side-by-side 뷰** — 적용된 설정(왼쪽, 빨간색)과 원하는 설정(오른쪽, 녹색)을 라인별 JSON diff로 표시합니다. Git diff와 유사한 라인 번호와 색상 하이라이팅을 제공합니다.

상단의 **Fields** / **Side-by-side** 토글 버튼으로 보기를 전환할 수 있습니다.

Drift가 감지되면 **Force Reconcile** 버튼으로 오퍼레이터에 재조정을 요청할 수 있습니다. 이 버튼은 CR에 `acko.io/force-reconcile` 어노테이션을 추가하여 즉시 reconciliation을 트리거합니다.

### Reconciliation Health & Circuit Breaker

Reconciliation 실패가 발생하면 **Reconciliation Health** 카드가 나타납니다:

- **Progress Bar** — 서킷 브레이커 임계값(10회)까지의 실패 진행도
- **Backoff Timer** — 서킷 브레이커 활성화 시 다음 재시도까지의 예상 시간
- **Error Details** — 마지막 reconciliation 에러 메시지
- **Reset Button** — Reconciliation Health 카드의 전용 **Reset** 버튼으로 서킷 브레이커를 원클릭으로 리셋합니다. 이전에는 `kubectl`로 수동 CR 패치가 필요했으나, 이제 `acko.io/reset-circuit-breaker` 어노테이션을 통한 즉시 리셋이 가능합니다.

서킷 브레이커는 연속 10회 실패 시 자동 활성화되며, 지수 백오프(30s × 2^n, 최대 300s)로 재시도합니다.

### PVC / Storage Status

클러스터 상세 페이지에서 **Storage (PVCs)** 카드가 해당 클러스터의 PersistentVolumeClaim 상태를 표시합니다:

- **Status Badge** — Bound (초록), Pending (노랑), Lost (빨강)
- **Capacity** — 프로비저닝된 스토리지 용량
- **Storage Class** — 사용된 Kubernetes StorageClass
- **Access Modes** — ReadWriteOnce, ReadWriteMany 등
- **Volume Name** — 바인딩된 PersistentVolume 이름
- **PVC 삭제** — 고아 PVC(활성 Pod와 연결되지 않은 PVC)를 PVC 상태 목록에서 직접 삭제할 수 있습니다. 스케일다운이나 Pod 삭제 실패 후 남은 스토리지 리소스를 정리하는 데 유용합니다. 삭제 전 확인 다이얼로그가 표시됩니다.

### Export / Import

**Export** — 클러스터 상세 페이지의 Spec 섹션에서 **Copy CR** 버튼으로 CR을 JSON 형식으로 클립보드에 복사합니다.

**Import** — 클러스터 리스트 페이지에서 **Import CR** 버튼으로 기존 CR JSON을 붙여넣기하거나 파일을 업로드하여 클러스터를 생성합니다. 메타데이터 필드(`uid`, `resourceVersion`, `managedFields`)는 자동으로 제거됩니다.

**Clone** — 클러스터 상세 페이지에서 **Clone** 버튼으로 기존 클러스터의 spec을 복사하여 새 클러스터를 생성합니다. 새 이름과 네임스페이스를 지정하면 `operations`와 `paused` 상태를 제외한 전체 spec이 복제됩니다. 프로덕션 클러스터의 설정을 테스트 환경에 복제하거나, 검증된 설정을 다른 네임스페이스에 배포할 때 유용합니다.

:::caution 다른 네임스페이스로 복제 시 ACL Secret
다른 네임스페이스로 클러스터를 복제할 때, ACL 사용자의 비밀번호에 연결된 Kubernetes Secret(`spec.acl.users[].secretName`)은 자동으로 복사되지 않습니다. 복제된 클러스터가 ACL이 활성화된 상태로 정상 시작하려면, 대상 네임스페이스에 동일한 Secret이 미리 존재해야 합니다.
:::

---

## Namespaces

**Namespaces** 탭에서 네임스페이스별 오브젝트 수, 스토리지 타입, 복제 계수, 메모리/디스크 HWM, TTL 설정을 확인합니다. 각 네임스페이스 하위 Set 목록도 표시됩니다.

![Namespaces](/img/ui/namespaces.png)

Set 행을 클릭하면 레코드 브라우저로 이동합니다. **Add filter**로 Secondary Index 기반 필터를 추가할 수 있습니다.

![Namespaces Set Browser](/img/ui/namespaces-set-browser.png)

---

## Indexes

**Indexes** 탭에서 Secondary Index 목록(Name, Namespace, Set, Bin, Type, State)을 확인하고 **+ Create Index** 버튼으로 새 인덱스를 생성합니다.

![Secondary Indexes](/img/ui/indexes.png)

---

## Record Browser

**Browser** 탭에서 Aerospike 레코드를 조회, 생성, 수정, 삭제할 수 있습니다.

- Namespace와 Set을 선택하여 레코드를 스캔
- 페이지네이션을 통한 대량 레코드 탐색
- 개별 레코드의 Bin 값을 인라인 편집
- PK(Primary Key)로 레코드 직접 조회
- Secondary Index 기반 필터를 추가하여 조건부 스캔

---

## AQL Terminal

**Terminal** 탭에서 Monaco Editor 기반의 AQL(Aerospike Query Language) 터미널을 제공합니다.

- AQL 명령어 직접 입력 및 실행
- 구문 하이라이팅 및 자동 완성
- 실행 결과를 테이블/JSON 형식으로 표시

---

## UDF Management

**UDFs** 탭에서 Lua User-Defined Function을 관리합니다.

- 등록된 UDF 모듈 목록 확인
- 새로운 Lua UDF 파일 업로드
- UDF 모듈 삭제

---

## User & Role Management

**Admin** 탭에서 Aerospike 접근 제어(ACL)를 관리합니다.

- 사용자 목록 조회, 생성, 삭제, 비밀번호 변경
- 역할(Role) 목록 조회, 생성, 삭제
- 역할별 권한(Privilege) 관리
- 사용자-역할 매핑

---

## Service Metadata

클러스터 생성 마법사의 **Advanced** 단계와 클러스터 편집 다이얼로그에서 Kubernetes 서비스에 사용자 정의 메타데이터를 추가할 수 있습니다.

### Headless Service Metadata

오퍼레이터가 생성하는 headless 서비스(`<cluster-name>-headless`)에 커스텀 annotations과 labels를 추가합니다. 이는 Prometheus 서비스 디스커버리, External DNS 통합, 또는 비용 추적에 유용합니다.

### Per-Pod Service Metadata

`podService`를 설정하면 각 Pod마다 개별 ClusterIP Service가 생성됩니다. 커스텀 annotations과 labels를 추가하여 External DNS 통합, 서비스 메시 연동, 또는 Pod 수준의 로드 밸런싱에 활용할 수 있습니다.

### Pod Metadata

Aerospike Pod 자체에 커스텀 labels와 annotations를 추가합니다. 서비스 메시 사이드카 주입(예: Istio), 모니터링 레이블 셀렉터, 비용 할당 태그에 활용됩니다.

---
## HPA (Horizontal Pod Autoscaler) Management

클러스터 상세 페이지에서 AerospikeCluster 리소스를 대상으로 하는 HorizontalPodAutoscaler(HPA)를 관리할 수 있습니다. HPA는 CPU 또는 메모리 사용량에 따라 클러스터 크기를 자동으로 조정합니다.

### HPA 생성

클러스터 상세 페이지의 작업 메뉴에서 **HPA 관리**를 선택하여 새 HPA를 생성합니다. 다음 항목을 설정할 수 있습니다:

- **최소 레플리카(Min Replicas)** — 자동 스케일링 시 유지할 최소 Pod 수
- **최대 레플리카(Max Replicas)** — 허용할 최대 Pod 수
- **CPU 목표 사용률(CPU Target Utilization)** — 스케일 아웃을 트리거하는 평균 CPU 사용률(%)
- **메모리 목표 사용률(Memory Target Utilization)** — 스케일 아웃을 트리거하는 평균 메모리 사용률(%)

생성된 HPA는 해당 AerospikeCluster를 `scaleTargetRef`로 참조합니다.

### HPA 조회

클러스터에 연결된 HPA가 존재하면 현재 레플리카 수, 목표 메트릭, 현재 메트릭 값을 확인할 수 있습니다.

### HPA 삭제

더 이상 자동 스케일링이 필요하지 않은 경우 UI에서 HPA를 삭제할 수 있습니다. 삭제 후에는 수동 스케일링으로 전환됩니다.

:::note
HPA 관리 기능을 사용하려면 UI의 ClusterRole에 `autoscaling` API 그룹에 대한 권한이 필요합니다. `ui.rbac.create=true`(기본값)일 때 자동으로 설정됩니다.
:::

---

## K8s Cluster Management

`ui.k8s.enabled=true`일 때, **K8s Clusters** 페이지에서 `AerospikeCluster` CR을 GUI로 관리합니다.

### Cluster List

모든 네임스페이스의 AerospikeCluster를 카드 형식으로 표시합니다. 각 카드에 Phase, 노드 수, 이미지, 생성 시간이 표시됩니다.

### Create Cluster Wizard

**Scratch Mode** (9단계) 또는 **Template Mode** (3단계)로 클러스터를 생성합니다:

1. **Creation Mode** — Scratch 또는 Template 선택
2. **Basic** — 이름, 네임스페이스, 이미지, 노드 수
3. **Namespace & Storage** — Aerospike 네임스페이스 및 볼륨 구성
4. **Monitoring & Options** — Prometheus, Dynamic Config, NetworkPolicy, Seeds Finder LB
5. **Resources** — CPU/Memory requests/limits
6. **Security & ACL** — 역할 및 사용자 구성
7. **Rolling Update** — 배치 크기, PDB, Max Unavailable
8. **Rack Config** — 랙별 zone/region 설정, rack revision (랙 단위 롤링 리스타트 트리거), per-rack storage overrides (다른 StorageClass/크기), per-rack tolerations/affinity/nodeSelector overrides
9. **Advanced** — Node selector, tolerations, bandwidth, readiness gate, pod metadata (labels/annotations), headless service metadata (annotations/labels), per-pod service metadata (annotations/labels)
10. **Review** — 전체 설정 확인 및 배포

### Cluster Detail

클러스터 선택 시 다음 정보와 작업이 제공됩니다:

- **Overview** — Phase, Health, Conditions, Pod 목록
- **Events Timeline** — 11개 카테고리별 필터링 가능한 K8s 이벤트, JSON/CSV 내보내기 지원
- **Config Drift Detection** — spec vs appliedSpec 비교, Fields/Side-by-side diff 뷰, Pod별 config hash 그룹핑, Force Reconcile 버튼
- **Reconciliation Health** — 서킷 브레이커 상태, 실패 횟수, 백오프 타이머
- **PVC / Storage Status** — PersistentVolumeClaim 상태 표시 (Bound/Pending/Lost, 용량, StorageClass)
- **Pod Logs** — 개별 Pod 로그 조회
- **JSON Export / Import** — CR을 클린 JSON으로 내보내기 (Copy CR), JSON 파일로부터 클러스터 가져오기
- **Operations** — Scale, Edit, Warm Restart, Pod Restart, Pause/Resume, Delete, HPA 관리, Template Resync

### Template Management

**K8s Templates** 페이지에서 cluster-scoped `AerospikeClusterTemplate` 리소스의 전체 라이프사이클을 관리합니다.

#### Creating Templates

**+ Create Template** 버튼으로 새 템플릿을 생성합니다. 마법사에서 다음 항목을 설정할 수 있습니다:

- **Basic** — 템플릿 이름, 기본 Aerospike 이미지, 기본 클러스터 크기
- **Resources** — CPU/Memory requests 및 limits
- **Storage** — 스토리지 클래스, 볼륨 크기, 로컬 PV 옵션
- **Scheduling** — Pod 스케줄링 제약 조건 (아래 참조)
- **Monitoring** — Prometheus exporter 사이드카, ServiceMonitor, PrometheusRule
- **Network** — 네트워크 접근 정책 (accessType, fabricType)
- **Aerospike Config** — 서비스 설정, 네임스페이스 기본값

#### Viewing Templates

템플릿 목록 페이지에서 각 템플릿 카드에 참조 클러스터 수(`usedBy` count)가 표시됩니다. 카드를 클릭하면 상세 페이지에서 전체 설정과 해당 템플릿을 참조하는 클러스터 목록을 확인할 수 있습니다.

#### Editing Templates (Patch/Update)

템플릿 상세 페이지에서 **Edit** 버튼으로 편집 다이얼로그를 열 수 있습니다. RBAC 수정을 통해 `AerospikeClusterTemplate` 리소스에 대한 `patch` 및 `update` 권한이 UI 서비스 어카운트에 부여되어, UI에서 직접 템플릿을 수정할 수 있습니다.

편집 가능한 필드:
- 기본 이미지 및 클러스터 크기
- 리소스 requests/limits
- 스토리지 설정
- 스케줄링 설정
- 모니터링 설정
- 네트워크 정책
- Aerospike 설정

#### Deleting Templates

참조 클러스터가 없는 템플릿만 삭제할 수 있습니다. 클러스터가 아직 참조 중인 경우, 먼저 해당 클러스터의 `templateRef`를 제거하거나 다른 템플릿으로 변경해야 합니다.

#### Template Scheduling Configuration

템플릿의 `scheduling` 섹션에서 다음 스케줄링 제약 조건을 설정할 수 있습니다:

| Field | Description |
|-------|-------------|
| `podAntiAffinityLevel` | Pod anti-affinity 수준: `none`, `preferred`, `required`. `required`이면 노드당 하나의 Aerospike Pod만 배치됩니다. |
| `tolerations` | Kubernetes tolerations 배열. 테인트가 있는 노드에서도 Pod를 스케줄링할 수 있습니다. |
| `nodeAffinity` | 노드 라벨 기반 스케줄링 제약 조건. 특정 노드 풀에 Pod를 배치합니다. |
| `topologySpreadConstraints` | 토폴로지 도메인(zone, region 등)에 걸쳐 Pod를 균등 분배합니다. |

#### Template Topology Spread Constraints

템플릿 생성/편집 마법사의 **Scheduling** 단계에서 `topologySpreadConstraints`를 설정하여 Pod를 토폴로지 도메인에 걸쳐 균등하게 분배할 수 있습니다. 각 제약 조건에 대해 다음 필드를 UI에서 구성합니다:

| Field | Description |
|-------|-------------|
| **maxSkew** | 토폴로지 도메인 간 허용되는 최대 Pod 수 차이. 값이 작을수록 더 균등하게 분배됩니다. |
| **topologyKey** | Pod를 분배할 기준이 되는 노드 라벨 키. 일반적인 값: `topology.kubernetes.io/zone` (가용 영역별), `kubernetes.io/hostname` (노드별). |
| **whenUnsatisfiable** | 제약 조건을 만족할 수 없을 때의 동작: `DoNotSchedule` (스케줄링 거부) 또는 `ScheduleAnyway` (최선의 노력으로 스케줄링). |
| **labelSelector** | 분배 대상 Pod를 선택하는 라벨 셀렉터. `matchLabels` 또는 `matchExpressions`로 지정합니다. |

이 설정은 해당 템플릿을 참조하는 모든 클러스터에 기본값으로 적용됩니다. 클러스터별로 `spec.overrides.scheduling.topologySpreadConstraints`를 사용하여 재정의할 수도 있습니다. 자세한 내용은 [Advanced Configuration — topologySpreadConstraints](advanced-configuration.md#topologyspreadconstraints)를 참조하세요.

#### Template Resync

템플릿을 수정한 후, 이를 참조하는 기존 클러스터는 자동으로 업데이트되지 않습니다. 클러스터 상세 페이지의 **Template Resync** 버튼을 클릭하면 최신 템플릿 설정을 클러스터에 다시 적용합니다. 내부적으로 `acko.io/resync-template=true` 어노테이션을 추가하여 오퍼레이터가 템플릿을 다시 가져오도록 트리거합니다.

---

## Configuration Options

| Parameter | Description | Default |
|-----------|-------------|---------|
| `ui.enabled` | UI 활성화 | `false` |
| `ui.replicaCount` | UI 레플리카 수 | `1` |
| `ui.image.repository` | UI 컨테이너 이미지 | `ghcr.io/aerospike-ce-ecosystem/aerospike-cluster-manager` |
| `ui.image.tag` | 이미지 태그 | `latest` |
| `ui.service.type` | 서비스 타입 | `ClusterIP` |
| `ui.service.frontendPort` | 프론트엔드 (Next.js) 포트 | `3000` |
| `ui.service.backendPort` | 백엔드 (FastAPI) 포트 | `8000` |
| `ui.service.annotations` | 서비스 어노테이션 (클라우드 LB 설정 등) | `{}` |
| `ui.ingress.enabled` | Ingress 생성 | `false` |
| `ui.persistence.enabled` | PostgreSQL PVC 사용 | `true` |
| `ui.persistence.size` | PVC 스토리지 크기 | `1Gi` |
| `ui.k8s.enabled` | K8s 클러스터 관리 기능 | `true` |
| `ui.rbac.create` | ClusterRole/Binding 자동 생성 (AerospikeCluster, Template, HPA 관리 권한 포함) | `true` |
| `ui.resources.requests.cpu` | UI 컨테이너 CPU 요청 | `100m` |
| `ui.resources.requests.memory` | UI 컨테이너 메모리 요청 | `256Mi` |
| `ui.resources.limits.cpu` | UI 컨테이너 CPU 제한 | `200m` |
| `ui.resources.limits.memory` | UI 컨테이너 메모리 제한 | `512Mi` |
| `ui.postgresql.enabled` | 내장 PostgreSQL 사이드카 배포 | `true` |
| `ui.env.databaseUrl` | 외부 PostgreSQL URL (`postgresql.enabled=false` 일 때) | `""` |
| `ui.env.corsOrigins` | 백엔드 CORS origins (빈 문자열 = CORS 비활성화; 프론트엔드가 Next.js rewrites로 프록시) | `""` |
| `ui.env.logLevel` | 로그 레벨 (`DEBUG`, `INFO`, `WARNING`, `ERROR`) | `"INFO"` |
| `ui.env.logFormat` | 로그 포맷: `"text"` (사람 친화적), `"json"` (구조화 로깅) | `"text"` |
| `ui.env.dbPoolSize` | DB 커넥션 풀 크기 | `5` |
| `ui.env.dbPoolOverflow` | 풀 크기 초과 시 최대 추가 커넥션 수 | `10` |
| `ui.env.dbPoolTimeout` | 풀에서 커넥션 획득 타임아웃 (초) | `30` |
| `ui.env.k8sApiTimeout` | Kubernetes API 요청 타임아웃 (초) | `30` |
| `ui.extraEnv` | UI 컨테이너에 추가할 환경 변수 목록 | `[]` |
| `ui.metrics.serviceMonitor.enabled` | UI 백엔드 메트릭용 ServiceMonitor 생성 | `false` |
| `ui.metrics.serviceMonitor.interval` | 메트릭 스크랩 주기 | `"30s"` |
| `ui.metrics.serviceMonitor.scrapeTimeout` | 스크랩 타임아웃 | `"10s"` |
| `ui.metrics.serviceMonitor.labels` | ServiceMonitor 추가 라벨 | `{}` |

전체 옵션 확인:

```bash
helm show values oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator | grep -A 500 "^ui:"
```

---

## UI Environment Variables

Helm 값으로 UI 백엔드의 환경 변수를 조정할 수 있습니다. 이 설정들은 `ui.env.*`로 노출됩니다.

### Database Connection Pool

내장 PostgreSQL 사이드카 또는 외부 PostgreSQL에 대한 커넥션 풀을 튜닝합니다:

| Parameter | Default | Description |
|-----------|---------|-------------|
| `ui.env.dbPoolSize` | `5` | 기본 커넥션 풀 크기. 동시 요청 수에 맞춰 조정합니다. |
| `ui.env.dbPoolOverflow` | `10` | 풀이 가득 찼을 때 추가로 생성 가능한 최대 커넥션 수. 순간 트래픽 급증 시 유용합니다. |
| `ui.env.dbPoolTimeout` | `30` | 풀에서 유휴 커넥션을 기다리는 최대 시간(초). 타임아웃 초과 시 에러를 반환합니다. |

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --namespace aerospike-operator --create-namespace \
  --set ui.enabled=true \
  --set ui.env.dbPoolSize=10 \
  --set ui.env.dbPoolOverflow=20 \
  --set ui.env.dbPoolTimeout=60
```

:::tip
동시 사용자가 많은 환경에서는 `dbPoolSize`를 늘려주세요. 일반적으로 `dbPoolSize`는 예상 동시 요청 수와 비슷하게, `dbPoolOverflow`는 그 2배 정도로 설정합니다.
:::

### Kubernetes API Timeout

UI가 Kubernetes API 서버에 요청할 때의 타임아웃을 설정합니다:

| Parameter | Default | Description |
|-----------|---------|-------------|
| `ui.env.k8sApiTimeout` | `30` | K8s API 요청 타임아웃(초). 대규모 클러스터에서 리스트 조회가 느린 경우 늘려주세요. |

### Logging

| Parameter | Default | Description |
|-----------|---------|-------------|
| `ui.env.logLevel` | `"INFO"` | 로그 레벨: `DEBUG`, `INFO`, `WARNING`, `ERROR` |
| `ui.env.logFormat` | `"text"` | `"text"`: 사람이 읽기 쉬운 형식, `"json"`: 구조화된 JSON 형식 (로그 수집 파이프라인과 연동 시 권장) |

```bash
# JSON 구조화 로깅 활성화 (Loki, Elasticsearch 등과 연동 시 권장)
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --namespace aerospike-operator --create-namespace \
  --set ui.enabled=true \
  --set ui.env.logFormat=json \
  --set ui.env.logLevel=INFO
```

---

## UI Metrics & ServiceMonitor

UI 백엔드는 `/metrics` 엔드포인트를 통해 Prometheus 메트릭을 노출합니다. Prometheus Operator를 사용하는 환경에서는 `ServiceMonitor`를 활성화하여 자동으로 메트릭을 수집할 수 있습니다.

:::note
ServiceMonitor는 Prometheus 기본 경로인 `/metrics`를 사용합니다. 별도의 `path` 설정은 필요하지 않습니다.
:::

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --namespace aerospike-operator --create-namespace \
  --set ui.enabled=true \
  --set ui.metrics.serviceMonitor.enabled=true \
  --set ui.metrics.serviceMonitor.labels.release=prometheus
```

| Parameter | Default | Description |
|-----------|---------|-------------|
| `ui.metrics.serviceMonitor.enabled` | `false` | ServiceMonitor 리소스 생성 여부 |
| `ui.metrics.serviceMonitor.interval` | `"30s"` | 메트릭 스크랩 주기 |
| `ui.metrics.serviceMonitor.scrapeTimeout` | `"10s"` | 스크랩 타임아웃 |
| `ui.metrics.serviceMonitor.labels` | `{}` | Prometheus 셀렉터 매칭을 위한 추가 라벨 |

:::tip
`labels.release=prometheus`는 Prometheus Operator의 `serviceMonitorSelector`와 일치해야 합니다. 다음 명령으로 확인하세요:
```bash
kubectl get prometheus -A -o jsonpath='{.items[*].spec.serviceMonitorSelector}'
```
:::

---

## Ingress (Production)

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

## Production Deployment Recommendations

프로덕션 환경에서 UI를 안정적으로 운영하기 위한 권장 사항입니다.

### External PostgreSQL

내장 PostgreSQL 사이드카는 단일 인스턴스로만 동작하며 HPA와 호환되지 않습니다. 프로덕션에서는 관리형 PostgreSQL(AWS RDS, GCP Cloud SQL, Azure Database for PostgreSQL 등)을 사용하세요.

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --namespace aerospike-operator --create-namespace \
  --set ui.enabled=true \
  --set ui.postgresql.enabled=false \
  --set ui.env.databaseUrl="postgresql://user:pass@rds-host:5432/aerospike_manager" \
  --set ui.env.dbPoolSize=10 \
  --set ui.env.dbPoolOverflow=20
```

:::warning
외부 PostgreSQL 사용 시 `ui.postgresql.enabled=false`로 설정하지 않으면 사이드카와 외부 DB가 동시에 생성됩니다.
:::

:::tip Database credentials in production
프로덕션 환경에서는 `--set ui.env.databaseUrl`에 평문 비밀번호를 넣는 대신, Kubernetes Secret을 사용하여 데이터베이스 자격 증명을 안전하게 관리하세요. `ui.extraEnv`를 활용하여 Secret에서 환경 변수를 주입할 수 있습니다:
```yaml
ui:
  extraEnv:
    - name: DATABASE_URL
      valueFrom:
        secretKeyRef:
          name: ui-db-credentials
          key: database-url
```
:::

### Monitoring

오퍼레이터와 UI 모두에 모니터링을 활성화하세요:

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --namespace aerospike-operator --create-namespace \
  --set ui.enabled=true \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.additionalLabels.release=prometheus \
  --set prometheusRule.enabled=true \
  --set grafanaDashboard.enabled=true \
  --set ui.metrics.serviceMonitor.enabled=true \
  --set ui.metrics.serviceMonitor.labels.release=prometheus
```

### High Availability

UI를 다중 레플리카로 운영하려면 반드시 외부 PostgreSQL을 사용해야 합니다:

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --namespace aerospike-operator --create-namespace \
  --set ui.enabled=true \
  --set ui.replicaCount=2 \
  --set ui.postgresql.enabled=false \
  --set ui.env.databaseUrl="postgresql://user:pass@rds-host:5432/aerospike_manager" \
  --set ui.podDisruptionBudget.enabled=true \
  --set ui.podDisruptionBudget.minAvailable=1
```

### Full Production Example

```bash
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --namespace aerospike-operator --create-namespace \
  --set ui.enabled=true \
  --set ui.replicaCount=2 \
  --set ui.postgresql.enabled=false \
  --set ui.env.databaseUrl="postgresql://user:pass@rds-host:5432/aerospike_manager" \
  --set ui.env.dbPoolSize=10 \
  --set ui.env.dbPoolOverflow=20 \
  --set ui.env.logFormat=json \
  --set ui.ingress.enabled=true \
  --set ui.ingress.className=nginx \
  --set "ui.ingress.hosts[0].host=aerospike-admin.example.com" \
  --set "ui.ingress.hosts[0].paths[0].path=/" \
  --set "ui.ingress.hosts[0].paths[0].pathType=Prefix" \
  --set ui.podDisruptionBudget.enabled=true \
  --set ui.podDisruptionBudget.minAvailable=1 \
  --set ui.metrics.serviceMonitor.enabled=true \
  --set ui.metrics.serviceMonitor.labels.release=prometheus \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.additionalLabels.release=prometheus \
  --set prometheusRule.enabled=true \
  --set grafanaDashboard.enabled=true
```
