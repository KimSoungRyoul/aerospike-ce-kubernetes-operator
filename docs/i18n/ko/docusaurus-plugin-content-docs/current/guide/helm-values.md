---
sidebar_position: 2.5
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
| `resources.limits.memory` | string | `256Mi` | 오퍼레이터 파드 메모리 제한. |
| `resources.requests.cpu` | string | `100m` | 오퍼레이터 파드 CPU 요청. |
| `resources.requests.memory` | string | `128Mi` | 오퍼레이터 파드 메모리 요청. |

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
| `podDisruptionBudget.enabled` | bool | `false` | 오퍼레이터 디플로이먼트용 PodDisruptionBudget 생성. |
| `podDisruptionBudget.minAvailable` | int | `1` | 최소 가용 파드 수. `maxUnavailable`과 상호 배타적. |
| `podDisruptionBudget.maxUnavailable` | int | — | 최대 비가용 파드 수. `minAvailable`과 상호 배타적. |

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

Aerospike Cluster Manager는 오퍼레이터와 함께 배포되는 풀스택 웹 대시보드입니다. Aerospike 클러스터를 모니터링하고 관리하기 위한 시각적 인터페이스를 제공합니다.

### 일반

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.enabled` | bool | `false` | Aerospike Cluster Manager 웹 UI 활성화. |
| `ui.replicaCount` | int | `1` | UI 레플리카 수. |
| `ui.image.repository` | string | `ghcr.io/aerospike-ce-ecosystem/aerospike-cluster-manager` | UI 컨테이너 이미지 리포지토리. |
| `ui.image.tag` | string | `"latest"` | UI 컨테이너 이미지 태그. UI는 오퍼레이터와 독립적으로 버전 관리. |
| `ui.image.pullPolicy` | string | `IfNotPresent` | 이미지 풀 정책. |
| `ui.imagePullSecrets` | list | `[]` | 프라이빗 레지스트리용 이미지 풀 시크릿. |

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

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.service.type` | string | `ClusterIP` | 서비스 타입: `ClusterIP`, `NodePort`, 또는 `LoadBalancer`. |
| `ui.service.frontendPort` | int | `3000` | 프론트엔드 포트 (Next.js 웹 UI). |
| `ui.service.backendPort` | int | `8000` | 백엔드 포트 (FastAPI REST API). |
| `ui.service.annotations` | object | `{}` | UI 서비스 어노테이션. |

### Ingress

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.ingress.enabled` | bool | `false` | 외부 접근용 Ingress 활성화. |
| `ui.ingress.className` | string | `""` | Ingress 클래스 이름. |
| `ui.ingress.annotations` | object | `{}` | Ingress 어노테이션. |
| `ui.ingress.hosts` | list | values.yaml 참조 | Ingress 호스트 규칙. |
| `ui.ingress.tls` | list | `[]` | Ingress TLS 설정. |

### PostgreSQL (내장 사이드카)

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.postgresql.enabled` | bool | `true` | 내장 PostgreSQL 사이드카 컨테이너 배포. 비활성화하여 외부 PostgreSQL 인스턴스 사용. |
| `ui.postgresql.image.repository` | string | `postgres` | PostgreSQL 컨테이너 이미지. |
| `ui.postgresql.image.tag` | string | `"17-alpine"` | PostgreSQL 이미지 태그. |
| `ui.postgresql.image.pullPolicy` | string | `IfNotPresent` | 이미지 풀 정책. |
| `ui.postgresql.database` | string | `aerospike_manager` | 데이터베이스 이름. |
| `ui.postgresql.username` | string | `aerospike` | 데이터베이스 사용자. |
| `ui.postgresql.password` | string | `aerospike` | 데이터베이스 비밀번호 (내장 사이드카 전용). |
| `ui.postgresql.existingSecret` | string | `""` | `POSTGRES_PASSWORD`와 `DATABASE_URL` 키를 포함하는 기존 Secret 이름. |
| `ui.postgresql.resources.requests.cpu` | string | `50m` | CPU 요청. |
| `ui.postgresql.resources.requests.memory` | string | `128Mi` | 메모리 요청. |
| `ui.postgresql.resources.limits.cpu` | string | `250m` | CPU 제한. |
| `ui.postgresql.resources.limits.memory` | string | `256Mi` | 메모리 제한. |

내장 PostgreSQL 사이드카에는 `pg_isready`를 실행하는 **startup probe**가 포함되어 있어, UI 컨테이너가 트래픽을 수신하기 전에 데이터베이스가 준비되었는지 확인합니다.

### 영속성

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.persistence.enabled` | bool | `true` | 내장 PostgreSQL 데이터베이스용 영구 스토리지 활성화. |
| `ui.persistence.storageClassName` | string | `""` | 스토리지 클래스 이름 (비어있으면 기본값). |
| `ui.persistence.accessMode` | string | `ReadWriteOnce` | 접근 모드. |
| `ui.persistence.size` | string | `1Gi` | 볼륨 크기. |

### 배포 전략 및 정상 종료

UI Deployment는 PostgreSQL 설정에 따라 명시적인 업데이트 전략을 사용합니다:

- **내장 PostgreSQL 사용 시** (`ui.postgresql.enabled=true`): PVC가 한 번에 하나의 Pod에만 마운트될 수 있으므로 `Recreate` 전략 사용.
- **외부 PostgreSQL 사용 시** (`ui.postgresql.enabled=false`): 무중단 배포를 위해 `RollingUpdate` 전략 사용 (`maxSurge: 1`, `maxUnavailable: 0`).

UI 컨테이너에는 **preStop 라이프사이클 훅** (`sleep 5`)이 포함되어 Pod 종료 전 진행 중인 요청이 완료될 수 있도록 합니다. `terminationGracePeriodSeconds` (기본값: 45)와 함께 롤아웃 및 노드 드레인 시 정상 종료를 보장합니다.

### K8s 클러스터 관리

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.k8s.enabled` | bool | `true` | Kubernetes 클러스터 관리 기능 활성화 (클러스터 생성). |

### UI 리소스

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.resources.requests.cpu` | string | `100m` | CPU 요청. |
| `ui.resources.requests.memory` | string | `256Mi` | 메모리 요청. |
| `ui.resources.limits.cpu` | string | `200m` | CPU 제한. |
| `ui.resources.limits.memory` | string | `512Mi` | 메모리 제한. |

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

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.livenessProbe.httpGet.path` | string | `/api/health` | Liveness 프로브 경로. |
| `ui.livenessProbe.httpGet.port` | string | `backend` | Liveness 프로브 포트. |
| `ui.livenessProbe.initialDelaySeconds` | int | `15` | 초기 대기 시간. |
| `ui.livenessProbe.periodSeconds` | int | `20` | 점검 주기. |
| `ui.livenessProbe.timeoutSeconds` | int | `5` | 타임아웃. |
| `ui.readinessProbe.httpGet.path` | string | `/api/health` | Readiness 프로브 경로. |
| `ui.readinessProbe.httpGet.port` | string | `backend` | Readiness 프로브 포트. |
| `ui.readinessProbe.initialDelaySeconds` | int | `5` | 초기 대기 시간. |
| `ui.readinessProbe.periodSeconds` | int | `10` | 점검 주기. |
| `ui.readinessProbe.timeoutSeconds` | int | `5` | 타임아웃. |
| `ui.startupProbe.httpGet.path` | string | `/api/health` | Startup 프로브 경로. |
| `ui.startupProbe.httpGet.port` | string | `backend` | Startup 프로브 포트. |
| `ui.startupProbe.periodSeconds` | int | `5` | 점검 주기. |
| `ui.startupProbe.timeoutSeconds` | int | `3` | 타임아웃. |
| `ui.startupProbe.failureThreshold` | int | `30` | 포기 전 최대 실패 횟수 (150초 시작 허용). |

### 환경 변수

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.env.frontendPort` | string | (`ui.service.frontendPort`에서 파생) | ConfigMap에 `FRONTEND_PORT`로 주입되는 프론트엔드 포트. 서비스 포트 설정에서 자동 파생. |
| `ui.env.backendPort` | string | (`ui.service.backendPort`에서 파생) | ConfigMap에 `BACKEND_PORT`로 주입되는 백엔드 포트. 서비스 포트 설정에서 자동 파생. |
| `ui.env.corsOrigins` | string | `""` | 백엔드 CORS 오리진. 비어있으면 CORS 없음 (프론트엔드가 Next.js rewrites로 프록시). |
| `ui.env.logLevel` | string | `"INFO"` | 로그 레벨: `DEBUG`, `INFO`, `WARNING`, `ERROR`. |
| `ui.env.logFormat` | string | `"text"` | 로그 형식: `text`(사람이 읽기 쉬운), `json`(구조화된 로깅). |
| `ui.env.databaseUrl` | string | `""` | 외부 PostgreSQL 연결 URL. `postgresql.enabled`가 `false`일 때만 사용. |
| `ui.env.dbPoolSize` | int | `5` | DB 커넥션 풀 크기. |
| `ui.env.dbPoolOverflow` | int | `10` | 풀 크기 초과 시 최대 오버플로 커넥션. |
| `ui.env.dbPoolTimeout` | int | `30` | 풀 체크아웃 타임아웃 (초). |
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
| `ui.extraEnv` | list | `[]` | UI 컨테이너의 추가 환경 변수. `valueFrom` 참조를 포함한 표준 Kubernetes 환경 변수 문법 지원. |

### UI Helm 테스트

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `ui.tests.enabled` | bool | `true` | UI용 Helm 테스트 파드 활성화 (`helm test <release>`로 실행). |

## 기본 AerospikeClusterTemplate

| 키 | 타입 | 기본값 | 설명 |
|-----|------|---------|-------------|
| `defaultTemplates.enabled` | bool | `true` | 사전 구축된 AerospikeClusterTemplate 리소스 생성 (minimal, soft-rack, hard-rack). 템플릿은 클러스터 범위이며 모든 네임스페이스에서 접근 가능. |

세 가지 기본 템플릿 티어는 `defaultTemplates.templates.minimal`, `defaultTemplates.templates.soft-rack`, `defaultTemplates.templates.hard-rack` 아래에 설정됩니다. 각 티어에 대한 자세한 내용은 [템플릿 관리](./templates.md)를 참조하세요.
