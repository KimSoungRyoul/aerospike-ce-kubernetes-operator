---
sidebar_position: 1
title: Helm Values 레퍼런스
---

# Helm Values 레퍼런스

이 페이지는 `aerospike-ce-kubernetes-operator` Helm 차트의 모든 설정 가능한 값을 문서화합니다.

## CRD 관리

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `crds.install` | bool | `true` | aerospike-ce-kubernetes-operator-crds를 서브차트 의존성으로 설치. CRD를 별도로 관리하는 경우(예: GitOps) `false`로 설정. |
| `crds.keep` | bool | `true` | `helm uninstall` 시 CRD 유지. 실제 유지 동작은 CRD 템플릿의 `helm.sh/resource-policy: keep` 어노테이션으로 적용됨. |

## 오퍼레이터

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `replicaCount` | int | `1` | 오퍼레이터 레플리카 수. 리더 선출이 HA를 처리하므로 일반적으로 1이면 충분. |
| `image.repository` | string | `ghcr.io/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator` | 오퍼레이터 컨테이너 이미지 리포지토리. |
| `image.tag` | string | `""` | 컨테이너 이미지 태그. 비어있으면 `Chart.appVersion` 사용. |
| `image.pullPolicy` | string | `IfNotPresent` | 이미지 풀 정책: `Always`, `IfNotPresent`, `Never`. |
| `imagePullSecrets` | list | `[]` | 프라이빗 레지스트리용 이미지 풀 시크릿. |
| `nameOverride` | string | `""` | 리소스 이름에 사용되는 차트 이름 오버라이드. |
| `fullnameOverride` | string | `""` | 전체 리소스 이름 오버라이드 (`nameOverride`보다 우선). |

## 서비스 어카운트

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `serviceAccount.annotations` | object | `{}` | 오퍼레이터 서비스 어카운트 어노테이션. IAM 역할(예: EKS IRSA, GKE Workload Identity)에 유용. |

## 리소스

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `resources.limits.cpu` | string | `500m` | 오퍼레이터 파드 CPU 제한. |
| `resources.limits.memory` | string | `512Mi` | 오퍼레이터 파드 메모리 제한. |
| `resources.requests.cpu` | string | `100m` | 오퍼레이터 파드 CPU 요청. |
| `resources.requests.memory` | string | `256Mi` | 오퍼레이터 파드 메모리 요청. |

## 웹훅

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `webhook.enabled` | bool | `true` | CR 검증 및 기본값 설정을 위한 admission 웹훅 활성화. |
| `webhook.port` | int | `9443` | 웹훅 서버 리슨 포트. |

## cert-manager 통합

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `certManager.enabled` | bool | `true` | cert-manager를 사용하여 웹훅 TLS 인증서를 프로비저닝. 비활성화 시 `webhookTlsSecret`을 통해 수동으로 TLS 시크릿 제공 필요. |
| `certManager.issuer.type` | string | `selfSigned` | Issuer 타입: `selfSigned`, `ca`, 또는 `clusterIssuer`. |
| `certManager.issuer.name` | string | `""` | 기존 ClusterIssuer 이름 (type이 `clusterIssuer`일 때만 사용). |
| `certManager.issuer.caSecretName` | string | `""` | `tls.crt`와 `tls.key`를 포함하는 CA 시크릿 이름 (type이 `ca`일 때만 사용). |
| `certManager.duration` | string | `""` | 인증서 유효 기간 (기본값: `8760h` = 1년). |
| `certManager.renewBefore` | string | `""` | 만료 전 인증서 갱신 시간 (기본값: `2880h` = 120일). |
| `webhookTlsSecret` | string | `""` | 웹훅 서버용 TLS 시크릿을 수동 제공. `certManager.enabled`가 `false`이고 `webhook.enabled`가 `true`일 때만 사용. 시크릿에 `tls.crt`와 `tls.key` 필요. |

## 모니터링 - ServiceMonitor

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `serviceMonitor.enabled` | bool | `false` | Prometheus Operator용 ServiceMonitor 리소스 생성. |
| `serviceMonitor.interval` | string | — | 스크래핑 주기 (예: `30s`). |
| `serviceMonitor.scrapeTimeout` | string | — | 스크래핑 타임아웃 (예: `10s`). |
| `serviceMonitor.additionalLabels` | object | `{}` | ServiceMonitor 디스커버리용 추가 레이블. |

## 모니터링 - PrometheusRule

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `prometheusRule.enabled` | bool | `false` | 오퍼레이터 알림 규칙이 포함된 PrometheusRule 리소스 생성. |
| `prometheusRule.additionalLabels` | object | `{}` | PrometheusRule 디스커버리용 추가 레이블. |
| `prometheusRule.rules` | list | `[]` | 기본값을 추가하거나 오버라이드할 커스텀 알림 규칙. 비어있으면 기본 규칙 사용. |

## 모니터링 - Grafana 대시보드

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `grafanaDashboard.enabled` | bool | `false` | 오퍼레이터용 Grafana 대시보드 ConfigMap 생성. Grafana 사이드카의 대시보드 자동 디스커버리 설정 필요. |
| `grafanaDashboard.sidecarLabel` | string | `grafana_dashboard` | Grafana 사이드카 대시보드 자동 디스커버리용 레이블 키. |
| `grafanaDashboard.sidecarLabelValue` | string | `"1"` | Grafana 사이드카 레이블 값. |
| `grafanaDashboard.folder` | string | `""` | 대시보드 정리용 Grafana 폴더 어노테이션. |

## Network Policy

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `networkPolicy.enabled` | bool | `false` | 표준 Kubernetes NetworkPolicy 리소스 생성. `cilium.enabled`와 상호 배타적. |

## Cilium Network Policy

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `cilium.enabled` | bool | `false` | 표준 NetworkPolicy 대신 CiliumNetworkPolicy 리소스 생성. `networkPolicy.enabled`와 상호 배타적. Cilium CNI 필요. |
| `cilium.l7Enabled` | bool | `false` | Aerospike 포트에 대한 L7(애플리케이션 레이어) 정책 규칙 활성화. |

## Pod Disruption Budget

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `podDisruptionBudget.enabled` | bool | `true` | 오퍼레이터 디플로이먼트용 PodDisruptionBudget 생성. |
| `podDisruptionBudget.minAvailable` | int | `""` | 최소 가용 파드 수. `maxUnavailable`과 상호 배타적. `replicaCount`가 1보다 클 때만 설정. |
| `podDisruptionBudget.maxUnavailable` | int | `1` | 최대 비가용 파드 수. `minAvailable`과 상호 배타적. |

## Horizontal Pod Autoscaler

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `autoscaling.enabled` | bool | `false` | 오퍼레이터 디플로이먼트용 HPA 활성화. 복수 레플리카 실행 시에만 유용. |
| `autoscaling.minReplicas` | int | `1` | 최소 레플리카 수. |
| `autoscaling.maxReplicas` | int | `3` | 최대 레플리카 수. |
| `autoscaling.targetCPUUtilizationPercentage` | int | `80` | 목표 평균 CPU 사용률. |
| `autoscaling.targetMemoryUtilizationPercentage` | int | — | 목표 평균 메모리 사용률 (선택사항). |

## 스케줄링

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `nodeSelector` | object | `{}` | 오퍼레이터 파드 스케줄링용 노드 셀렉터 레이블. |
| `tolerations` | list | `[]` | 오퍼레이터 파드 스케줄링용 toleration. |
| `affinity` | object | `{}` | 오퍼레이터 파드 스케줄링용 어피니티 규칙. |
| `topologySpreadConstraints` | list | `[]` | 오퍼레이터 파드 스케줄링용 토폴로지 분산 제약. |
| `priorityClassName` | string | `""` | 오퍼레이터 파드용 우선순위 클래스 이름. |

## 추가 어노테이션 및 레이블

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `podAnnotations` | object | `{}` | 오퍼레이터 파드의 추가 어노테이션. |
| `podLabels` | object | `{}` | 오퍼레이터 파드의 추가 레이블. |

## UI - Aerospike Cluster Manager

Aerospike Cluster Manager는 오퍼레이터와 함께 두 개의 독립적인 Deployment — `api`(FastAPI 백엔드)와 `web`(Next.js 프론트엔드) — 로 배포되는 풀스택 웹 대시보드입니다. Aerospike 클러스터를 모니터링하고 관리하기 위한 시각적 인터페이스를 제공합니다.

### 일반

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.api.enabled` | bool | `true` | Cluster Manager API (FastAPI) 컴포넌트 배포. `ui.web.enabled=false`와 함께 false로 설정하면 UI를 완전히 끔. |
| `ui.web.enabled` | bool | `true` | Cluster Manager web (Next.js) 컴포넌트 배포. `ui.api.enabled=false`와 함께 false로 설정하면 UI를 완전히 끔. |
| `ui.replicaCount` | int | `1` | 각 UI Deployment(api / web)의 레플리카 수. |
| `ui.imageTag` | string | `"latest"` | api/web 두 컴포넌트의 기본 이미지 태그. `aerospike-cluster-manager`는 오퍼레이터와 독립적으로 더 자주 릴리스되므로 기본값 `latest`로 새 설치가 ACKO 릴리스 없이도 최신 ACM 이미지를 추적. 재현 가능한 배포가 필요하면 특정 버전(예: `"0.30.0"`)으로 핀. Pod `pullPolicy`는 mutable 태그 재페치를 위해 기본 `Always`. |
| `ui.api.image.repository` | string | `ghcr.io/aerospike-ce-ecosystem/aerospike-cluster-manager-api` | API(FastAPI) 컨테이너 이미지 리포지토리. |
| `ui.api.image.tag` | string | `""` | API 이미지 태그. 비어 있으면 `ui.imageTag` 사용. |
| `ui.api.image.pullPolicy` | string | `Always` | API 이미지 풀 정책. `ui.imageTag`가 `latest`(mutable)이라 기본 `Always`. immutable 태그로 핀하면 `IfNotPresent`로 변경 권장. |
| `ui.web.image.repository` | string | `ghcr.io/aerospike-ce-ecosystem/aerospike-cluster-manager-web` | Web(Next.js) 컨테이너 이미지 리포지토리. |
| `ui.web.image.tag` | string | `""` | Web 이미지 태그. 비어 있으면 `ui.imageTag` 사용. |
| `ui.web.image.pullPolicy` | string | `Always` | Web 이미지 풀 정책. `ui.imageTag`가 `latest`(mutable)이라 기본 `Always`. immutable 태그로 핀하면 `IfNotPresent`로 변경 권장. |
| `ui.imagePullSecrets` | list | `[]` | 프라이빗 레지스트리용 이미지 풀 시크릿 (모든 UI Deployment에 적용). |

### 서비스 어카운트 & RBAC

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.serviceAccount.create` | bool | `true` | UI용 서비스 어카운트 생성. |
| `ui.serviceAccount.annotations` | object | `{}` | UI 서비스 어카운트 어노테이션. |
| `ui.rbac.create` | bool | `true` | K8s API 접근을 위한 ClusterRole 및 ClusterRoleBinding 생성. |

`ui.rbac.create=true`일 때 생성되는 ClusterRole에는 다음 권한이 포함됩니다:

| API 그룹 | 리소스 | 동작 |
|-----------|-----------|-------|
| `acko.io` | `aerospikeclusters`, `aerospikeclustertemplates` | get, list, watch, create, update, patch, delete |
| `acko.io` | `aerospikeclusters/status` | get |
| `acko.io` | `aerospikeclustertemplates/status` | get |
| `""` (core) | `pods`, `services`, `persistentvolumeclaims` | delete, get, list, watch |
| `""` (core) | `pods/log` | get |
| `""` (core) | `configmaps` | get, list, watch |
| `""` (core) | `secrets` | list |
| `""` (core) | `persistentvolumes` | get, list |
| `""` (core) | `nodes` | get, list |
| `""` (core) | `events` | get, list, watch |
| `""` (core) | `namespaces` | create, list |
| `storage.k8s.io` | `storageclasses` | list |
| `autoscaling` | `horizontalpodautoscalers` | get, list, watch, create, update, patch, delete |

### 서비스

UI 컴포넌트별로 각각의 서비스가 생성됩니다.

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.api.service.type` | string | `ClusterIP` | API 서비스 타입: `ClusterIP`, `NodePort`, 또는 `LoadBalancer`. |
| `ui.api.service.port` | int | `80` | API 서비스 포트. 트래픽은 컨테이너 포트 8000으로 전달됨. |
| `ui.api.service.targetPort` | int | `8000` | API 컨테이너 포트 (non-root 실행이므로 비특권 포트). |
| `ui.api.service.annotations` | object | `{}` | API 서비스 어노테이션. |
| `ui.web.service.type` | string | `ClusterIP` | Web 서비스 타입: `ClusterIP`, `NodePort`, 또는 `LoadBalancer`. |
| `ui.web.service.port` | int | `3100` | Web 서비스 포트. 브라우저 접근 시 port-forward(또는 Ingress 라우팅) 대상 포트. |
| `ui.web.service.annotations` | object | `{}` | Web 서비스 어노테이션. |
| `ui.web.env.apiUrl` | string | `""` | 외부 API URL. 설정 시 web 파드의 `proxy.js`가 in-cluster API 서비스 대신 이 주소로 `/api/*`를 전달. `ui.api.enabled=false`일 때 필수. |

### Ingress

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.ingress.enabled` | bool | `false` | 외부 접근용 Ingress 활성화. |
| `ui.ingress.target` | string | `web` | Ingress가 라우팅할 UI 서비스: `web` 또는 `api`. |
| `ui.ingress.className` | string | `""` | Ingress 클래스 이름. |
| `ui.ingress.annotations` | object | `{}` | Ingress 어노테이션. |
| `ui.ingress.hosts` | list | values.yaml 참조 | Ingress 호스트 규칙. |
| `ui.ingress.tls` | list | `[]` | Ingress TLS 설정. |

### 데이터베이스

api는 클러스터 연결 메타데이터를 데이터베이스에 저장합니다. 백엔드는 `ui.database.type`으로 선택하며, `sqlite`(내장, 기본값) 또는 `postgresql` 중 하나입니다. `postgresql`은 외부 인스턴스에 연결하거나, `ui.database.postgresql.deploy=true`로 차트가 단일 레플리카 PostgreSQL Deployment를 직접 프로비저닝하도록 할 수 있습니다. 이전 차트 버전의 임베디드 PostgreSQL *사이드카*는 제거되었습니다.

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.database.type` | string | `sqlite` | 데이터베이스 백엔드: `sqlite`(PVC에 저장되는 내장 파일) 또는 `postgresql`(외부 인스턴스). |
| `ui.database.acknowledgeEmbeddedPostgresRemoval` | bool | `false` | 업그레이드 안전 게이트. 임베디드 PostgreSQL Secret이 남아 있는 릴리스에서의 업그레이드를 이 값이 `true`가 될 때까지 차단. 임베디드 데이터를 백업한 뒤에만 설정(아래 마이그레이션 안내 참조). 신규 설치에는 영향 없음. |
| `ui.database.sqlite.persistence.enabled` | bool | `true` | SQLite 데이터베이스 파일을 PVC에 영속 저장. `false`이면 `emptyDir`을 사용하며 Pod 재시작 시 저장된 연결이 사라짐. |
| `ui.database.sqlite.persistence.storageClassName` | string | `null` | 스토리지 클래스. `null` = 클러스터 기본 StorageClass, `""` = 사전 프로비저닝된 PV, `"name"` = 지정한 StorageClass. |
| `ui.database.sqlite.persistence.accessMode` | string | `ReadWriteOnce` | PVC 접근 모드. SQLite는 단일 라이터이므로 `ReadWriteOnce` 유지. |
| `ui.database.sqlite.persistence.size` | string | `1Gi` | SQLite PVC 볼륨 크기. |
| `ui.database.postgresql.databaseUrl` | string | `""` | 외부 PostgreSQL 인스턴스의 연결 URL. `type=postgresql`일 때 필수(또는 `existingSecret`). |
| `ui.database.postgresql.existingSecret` | string | `""` | 데이터베이스 자격 증명을 담은 기존 Secret. `deploy=false`이면 `DATABASE_URL` 키가, `deploy=true`이면 `POSTGRES_PASSWORD`와 `DATABASE_URL` 키가 모두 필요. `deploy=true`에서 지정하면 차트가 자체 Secret을 렌더링하지 않음(`helm template` 렌더 간 비밀번호를 안정적으로 유지하는 GitOps 안전 방식). |
| `ui.database.postgresql.poolMinSize` | int | `2` | 커넥션 풀 최소 크기 (`DB_POOL_MIN_SIZE`). |
| `ui.database.postgresql.poolMaxSize` | int | `10` | 커넥션 풀 최대 크기 (`DB_POOL_MAX_SIZE`). |
| `ui.database.postgresql.commandTimeout` | int | `30` | SQL 명령 실행 타임아웃 (초, `DB_COMMAND_TIMEOUT`). |

아래 키들은 **차트 관리형** PostgreSQL을 프로비저닝하며 `ui.database.postgresql.deploy=true`일 때만 적용됩니다.

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.database.postgresql.deploy` | bool | `false` | `true`이면 차트가 단일 레플리카 PostgreSQL Deployment + Service + 데이터 PVC + Secret을 프로비저닝하고 api의 `DATABASE_URL`을 자동 연결. `databaseUrl`은 거부되며, 자체 자격 증명 Secret을 쓰려면 `existingSecret`을 설정. `false`이면 외부 인스턴스에 연결. |
| `ui.database.postgresql.acknowledgeStatefulSetMigration` | bool | `false` | `deploy=true` 업그레이드 안전 게이트. 차트 v1.6.0은 chart-managed PostgreSQL을 StatefulSet으로 실행했으나 이제 데이터 PVC 이름이 다른 Deployment로 실행됨. 남아 있는 StatefulSet을 감지하면 이 값이 `true`가 될 때까지 업그레이드를 차단(먼저 데이터베이스를 백업 — 아래 마이그레이션 안내 참조). 신규 설치에는 영향 없음. |
| `ui.database.postgresql.image.repository` | string | `postgres` | 차트 관리형 PostgreSQL 이미지 리포지토리. |
| `ui.database.postgresql.image.tag` | string | `"17"` | PostgreSQL 이미지 태그. |
| `ui.database.postgresql.image.pullPolicy` | string | `IfNotPresent` | PostgreSQL 이미지 풀 정책. |
| `ui.database.postgresql.auth.database` | string | `aerospike_manager` | 첫 시작 시 생성되는 데이터베이스 이름. |
| `ui.database.postgresql.auth.username` | string | `aerospike` | 데이터베이스 사용자. |
| `ui.database.postgresql.auth.password` | string | `""` | 데이터베이스 비밀번호. 비우면 첫 설치 시 24자 무작위 비밀번호를 생성하고 `helm upgrade` 시에도 유지. GitOps 주의: 클라이언트사이드 `helm template`(ArgoCD / Flux / kustomize)은 Secret을 다시 읽지 못해 빈 비밀번호가 매 렌더마다 재생성됨 — GitOps에서는 이 값을 명시하거나 `existingSecret`을 사용. |
| `ui.database.postgresql.persistence.enabled` | bool | `true` | PostgreSQL 데이터 디렉터리를 PVC에 영속 저장(`helm.sh/resource-policy: keep` — `helm uninstall` 후에도 유지). `false`이면 `emptyDir`을 사용하며 Pod 재시작 시 데이터가 모두 사라짐. |
| `ui.database.postgresql.persistence.storageClassName` | string | `null` | PostgreSQL PVC 스토리지 클래스. `null` = 클러스터 기본값, `""` = 사전 프로비저닝 PV, `"name"` = 지정 StorageClass. |
| `ui.database.postgresql.persistence.accessMode` | string | `ReadWriteOnce` | PostgreSQL PVC 접근 모드. |
| `ui.database.postgresql.persistence.size` | string | `8Gi` | PostgreSQL PVC 볼륨 크기. |
| `ui.database.postgresql.resources` | object | `100m`/`256Mi` → `500m`/`512Mi` | PostgreSQL 컨테이너 리소스 requests / limits. |
| `ui.database.postgresql.podSecurityContext` | object | `runAsUser/runAsGroup/fsGroup: 999` | PostgreSQL 파드 레벨 securityContext. |
| `ui.database.postgresql.securityContext` | object | 모든 capability 제거 | PostgreSQL 컨테이너 레벨 securityContext. |
| `ui.database.postgresql.nodeSelector` | object | `{}` | PostgreSQL 파드 노드 셀렉터. |
| `ui.database.postgresql.tolerations` | list | `[]` | PostgreSQL 파드 toleration. |
| `ui.database.postgresql.affinity` | object | `{}` | PostgreSQL 파드 어피니티. |

**SQLite (기본값)** 는 api 컨테이너 내부의 단일 파일에 데이터를 저장하며 PersistentVolumeClaim으로 백업됩니다. 단일 라이터이므로 `ui.replicaCount`는 `1`이어야 합니다 — 그렇지 않으면 차트가 설치를 실패시킵니다.

**PostgreSQL** 모드는 두 가지 하위 모드를 가집니다. `deploy=false`(기본값)이면 직접 운영하는 **외부** 데이터베이스(RDS / Cloud SQL / AlloyDB 같은 관리형 서비스, 또는 클러스터 내 PostgreSQL 오퍼레이터)에 연결하며 HA·다중 레플리카에 적합합니다. `deploy=true`이면 차트가 **단일 레플리카** PostgreSQL Deployment(`Recreate` 전략, 데이터는 `keep` 정책 PVC)를 프로비저닝하고 api를 자동 연결합니다 — 간편하지만 고가용성은 아닙니다. GitOps에서는 `ui.database.postgresql.auth.password`를 명시하거나 `ui.database.postgresql.existingSecret`을 직접 관리하는 Secret에 지정해, 클라이언트사이드 `helm template`이 매 렌더마다 비밀번호를 재생성하지 않도록 하세요.

> **마이그레이션:** 임베디드 사이드카 시절의 구버전 `ui.postgresql.*` / `ui.persistence.*` 키는 설치 시 마이그레이션 안내 메시지와 함께 실패합니다. `ui.postgresql.enabled: true` → `ui.database.type: postgresql` + `ui.database.postgresql.databaseUrl`, `ui.postgresql.enabled: false` → `ui.database.type: sqlite`, `ui.persistence.*` → `ui.database.sqlite.persistence.*`, `ui.env.databaseUrl` → `ui.database.postgresql.databaseUrl`로 매핑하세요. 임베디드 PostgreSQL에서 자동 데이터 마이그레이션은 제공되지 않으며, 임베디드 사이드카 릴리스에서의 업그레이드는 `ui.database.acknowledgeEmbeddedPostgresRemoval=true`를 설정할 때까지 차단됩니다. `pg_dump` → 복원 → 업그레이드 런북은 차트 README의 "Migrating off the embedded PostgreSQL sidecar" 절을 참고하세요.

### 배포 전략 및 정상 종료

api Deployment는 데이터베이스 백엔드에 따라 명시적인 업데이트 전략을 사용합니다:

- **SQLite** (`ui.database.type=sqlite`): SQLite PVC는 `ReadWriteOnce`이며 한 번에 하나의 Pod에만 마운트될 수 있으므로 `Recreate` 전략 사용.
- **PostgreSQL** (`ui.database.type=postgresql`): 무중단 배포를 위해 `RollingUpdate` 전략 사용 (`maxSurge: 1`, `maxUnavailable: 0`).

UI 컨테이너에는 **preStop 라이프사이클 훅** (`sleep 5`)이 포함되어 Pod 종료 전 진행 중인 요청이 완료될 수 있도록 합니다. `terminationGracePeriodSeconds` (기본값: 45)와 함께 롤아웃 및 노드 드레인 시 정상 종료를 보장합니다.

### K8s 클러스터 관리

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.k8s.enabled` | bool | `true` | Kubernetes 클러스터 관리 기능 활성화 (클러스터 생성). |
| `ui.k8s.verifySsl` | bool | `true` | Kubernetes API 서버 연결 시 TLS 인증서 검증. 자체 서명 또는 비표준 CA 인증서를 쓰는 클러스터에서는 `false`로 설정 가능. |
| `ui.k8s.caFile` | string | `""` | `ui.api.extraVolumes` / `ui.api.extraVolumeMounts`로 마운트한 Kubernetes API 커스텀 CA 번들의 api 컨테이너 내부 경로. |

### UI 리소스

api와 web Deployment는 각각 독립적인 리소스 설정을 가집니다.

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.api.resources.requests.cpu` | string | `100m` | API CPU 요청. |
| `ui.api.resources.requests.memory` | string | `256Mi` | API 메모리 요청. |
| `ui.api.resources.limits.cpu` | string | `200m` | API CPU 제한. |
| `ui.api.resources.limits.memory` | string | `512Mi` | API 메모리 제한. |
| `ui.web.resources.requests.cpu` | string | `50m` | Web CPU 요청. |
| `ui.web.resources.requests.memory` | string | `128Mi` | Web 메모리 요청. |
| `ui.web.resources.limits.cpu` | string | `150m` | Web CPU 제한. |
| `ui.web.resources.limits.memory` | string | `384Mi` | Web 메모리 제한. |

### 보안 컨텍스트

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.podSecurityContext.runAsNonRoot` | bool | `true` | non-root로 파드 실행. |
| `ui.podSecurityContext.runAsUser` | int | `1001` | 사용자 ID. |
| `ui.podSecurityContext.fsGroup` | int | `1001` | 파일시스템 그룹 ID. |
| `ui.podSecurityContext.seccompProfile.type` | string | `RuntimeDefault` | Seccomp 프로파일 타입. |
| `ui.securityContext.allowPrivilegeEscalation` | bool | `false` | 권한 상승 불허. |
| `ui.securityContext.readOnlyRootFilesystem` | bool | `false` | 읽기 전용 루트 파일시스템. |
| `ui.securityContext.capabilities.drop` | list | `["ALL"]` | 모든 Linux 기능 드롭. |

### 프로브

api Deployment는 liveness / readiness / startup 프로브를, web Deployment는 liveness / readiness 프로브를 가집니다.

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.api.livenessProbe.httpGet.path` | string | `/api/health` | API liveness 프로브 경로. |
| `ui.api.livenessProbe.httpGet.port` | string | `api` | API liveness 프로브 포트. |
| `ui.api.livenessProbe.initialDelaySeconds` | int | `15` | 초기 대기 시간. |
| `ui.api.livenessProbe.periodSeconds` | int | `20` | 점검 주기. |
| `ui.api.livenessProbe.timeoutSeconds` | int | `5` | 타임아웃. |
| `ui.api.readinessProbe.httpGet.path` | string | `/api/health` | API readiness 프로브 경로. |
| `ui.api.readinessProbe.httpGet.port` | string | `api` | API readiness 프로브 포트. |
| `ui.api.readinessProbe.initialDelaySeconds` | int | `5` | 초기 대기 시간. |
| `ui.api.readinessProbe.periodSeconds` | int | `10` | 점검 주기. |
| `ui.api.readinessProbe.timeoutSeconds` | int | `5` | 타임아웃. |
| `ui.api.startupProbe.httpGet.path` | string | `/api/health` | API startup 프로브 경로. |
| `ui.api.startupProbe.httpGet.port` | string | `api` | API startup 프로브 포트. |
| `ui.api.startupProbe.periodSeconds` | int | `5` | 점검 주기. |
| `ui.api.startupProbe.timeoutSeconds` | int | `3` | 타임아웃. |
| `ui.api.startupProbe.failureThreshold` | int | `30` | 포기 전 최대 실패 횟수 (150초 시작 허용). |
| `ui.web.livenessProbe.httpGet.path` | string | `/` | Web liveness 프로브 경로. |
| `ui.web.livenessProbe.httpGet.port` | string | `web` | Web liveness 프로브 포트. |
| `ui.web.livenessProbe.initialDelaySeconds` | int | `15` | 초기 대기 시간. |
| `ui.web.livenessProbe.periodSeconds` | int | `20` | 점검 주기. |
| `ui.web.livenessProbe.timeoutSeconds` | int | `5` | 타임아웃. |
| `ui.web.readinessProbe.httpGet.path` | string | `/` | Web readiness 프로브 경로. |
| `ui.web.readinessProbe.httpGet.port` | string | `web` | Web readiness 프로브 포트. |
| `ui.web.readinessProbe.initialDelaySeconds` | int | `5` | 초기 대기 시간. |
| `ui.web.readinessProbe.periodSeconds` | int | `10` | 점검 주기. |
| `ui.web.readinessProbe.timeoutSeconds` | int | `5` | 타임아웃. |

### 환경 변수

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.env.corsOrigins` | string | `""` | 백엔드 CORS 오리진. 비어있으면 CORS 없음 (web 파드가 `/api/*`를 프록시). |
| `ui.env.logLevel` | string | `"INFO"` | 로그 레벨: `DEBUG`, `INFO`, `WARNING`, `ERROR`. |
| `ui.env.logFormat` | string | `"text"` | 로그 형식: `text`(사람이 읽기 쉬운), `json`(구조화된 로깅). |
| `ui.env.k8sApiTimeout` | int | `30` | Kubernetes API 요청 타임아웃 (초). |

### UI 모니터링

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.metrics.serviceMonitor.enabled` | bool | `false` | UI 백엔드 메트릭 엔드포인트용 ServiceMonitor 생성. |
| `ui.metrics.serviceMonitor.interval` | string | `30s` | 스크래핑 주기. |
| `ui.metrics.serviceMonitor.scrapeTimeout` | string | `10s` | 스크래핑 타임아웃. |
| `ui.metrics.serviceMonitor.labels` | object | `{}` | ServiceMonitor 디스커버리용 추가 레이블. |

UI ServiceMonitor는 `/api/metrics` 경로에서 백엔드 메트릭을 스크래핑합니다. 이 경로는 Prometheus가 애플리케이션 레벨 메트릭을 올바르게 수집하도록 ServiceMonitor 템플릿에 명시적으로 설정됩니다.

### UI 스케줄링

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.nodeSelector` | object | `{}` | UI 파드용 노드 셀렉터. |
| `ui.tolerations` | list | `[]` | UI 파드용 toleration. |
| `ui.affinity` | object | `{}` | UI 파드용 어피니티 규칙. |
| `ui.topologySpreadConstraints` | list | `[]` | UI 파드용 토폴로지 분산 제약. |
| `ui.podAnnotations` | object | `{}` | UI 파드의 추가 어노테이션. |
| `ui.podLabels` | object | `{}` | UI 파드의 추가 레이블. |
| `ui.terminationGracePeriodSeconds` | int | `45` | 종료 유예 기간 (초). |

### UI Aerospike 포트

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.aerospikePorts.service` | int | `3000` | Aerospike 서비스 포트. |
| `ui.aerospikePorts.fabric` | int | `3001` | Aerospike 패브릭 포트. |
| `ui.aerospikePorts.heartbeat` | int | `3002` | Aerospike 하트비트 포트. |

### UI Network Policy

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.networkPolicy.enabled` | bool | `false` | UI 트래픽 제한용 NetworkPolicy 활성화. |
| `ui.networkPolicy.ingressFrom` | list | `[]` | 선택적 인그레스 소스 제한. |

### UI Pod Disruption Budget

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.podDisruptionBudget.enabled` | bool | `false` | UI 파드용 PDB 활성화. |
| `ui.podDisruptionBudget.minAvailable` | int | `1` | 최소 가용 파드 수. |
| `ui.podDisruptionBudget.maxUnavailable` | int | — | 최대 비가용 파드 수. |

### UI 오토스케일링

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.autoscaling.enabled` | bool | `false` | UI용 HPA 활성화. |
| `ui.autoscaling.minReplicas` | int | `1` | 최소 레플리카 수. |
| `ui.autoscaling.maxReplicas` | int | `3` | 최대 레플리카 수. |
| `ui.autoscaling.targetCPUUtilizationPercentage` | int | `80` | 목표 CPU 사용률. |
| `ui.autoscaling.targetMemoryUtilizationPercentage` | int | — | 목표 메모리 사용률 (선택사항). |

### 추가 환경 변수

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.extraEnv` | list | `[]` | 두 UI 컨테이너(api와 web)에 모두 주입되는 추가 환경 변수. `valueFrom` 참조를 포함한 표준 Kubernetes 환경 변수 문법 지원. |
| `ui.api.extraEnv` | list | `[]` | api 컨테이너에만 주입되는 추가 환경 변수. |
| `ui.api.extraEnvFrom` | list | `[]` | api 환경 변수를 일괄 주입하기 위한 `envFrom` 항목(`configMapRef` / `secretRef`). |

### UI Helm 테스트

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.tests.enabled` | bool | `true` | UI용 Helm 테스트 파드 활성화 (`helm test <release>`로 실행). |

## 기본 AerospikeClusterTemplate

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `defaultTemplates.enabled` | bool | `false` | 사전 구축된 AerospikeClusterTemplate 리소스 생성 (minimal, soft-rack, hard-rack). 템플릿은 AerospikeClusterTemplate CRD가 먼저 등록되어야 하므로 기본값은 `false` — 최초 설치 후 `helm upgrade`로 활성화. 템플릿은 클러스터 범위이며 모든 네임스페이스에서 접근 가능. |

세 가지 기본 템플릿 티어는 `defaultTemplates.templates.minimal`, `defaultTemplates.templates.soft-rack`, `defaultTemplates.templates.hard-rack` 아래에 설정됩니다. 각 티어에 대한 자세한 내용은 [템플릿 관리](../configuration/templates.md)를 참조하세요.
