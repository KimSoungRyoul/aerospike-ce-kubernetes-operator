---
sidebar_position: 1
title: 설치
---

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# 설치

Helm OCI 차트 또는 local chart checkout에서 ACKO를 설치합니다.

## 사전 준비

- Kubernetes 클러스터 v1.26+
- 대상 클러스터에 연결하도록 설정한 `kubectl`
- webhook TLS를 위한 [cert-manager](https://cert-manager.io/)

### cert-manager

Admission webhook이 TLS를 사용할 수 있도록 ACKO보다 cert-manager를 먼저 설치합니다.

```bash
helm repo add jetstack https://charts.jetstack.io
helm repo update
helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true
```

cert-manager가 준비될 때까지 기다립니다.

```bash
kubectl -n cert-manager wait --for=condition=Available deployment/cert-manager --timeout=60s
kubectl -n cert-manager wait --for=condition=Available deployment/cert-manager-webhook --timeout=60s
```

## 오퍼레이터 설치

<Tabs groupId="install-method">
<TabItem value="helm-oci" label="Helm OCI (권장)" default>

게시된 OCI Helm 차트를 사용하는 가장 간단한 설치 방법입니다.

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace
```

### Helm 값 커스터마이징

필요하면 기본값을 재정의합니다.

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set replicaCount=2 \
  --set resources.limits.memory=256Mi
```

사용 가능한 모든 값 조회:

```bash
helm show values oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator
```

### Cluster-admin 사전 설치 (RBAC 전용) {#cluster-admin-pre-install-rbac-only}

제한된(restricted) / 멀티테넌트 클러스터에서는 권한이 높은 **클러스터 범위(cluster-scoped)** 리소스 — 오퍼레이터의 `ClusterRole`/`ClusterRoleBinding`("cluster admin" 권한) — 를 플랫폼 팀이 네임스페이스 범위 워크로드 및 CRD(GitOps로 관리될 수 있음)와 분리해 설치하는 경우가 많습니다. `rbac.create` 값이 이 분리를 지원합니다.

- `rbac.create`의 기본값은 `null`이며, 이는 **`operator.enabled`를 따라갑니다** — 따라서 기존 설치는 영향을 받지 않습니다.
- 값을 명시하면 클러스터 범위 RBAC와 오퍼레이터 워크로드를 분리할 수 있습니다.

```bash
# Step 1 (cluster-admin, 한 번만): 클러스터 범위 RBAC만 설치.
#   CRD 없음, 오퍼레이터 Deployment 없음, webhook 없음, UI 없음.
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set operator.enabled=false \
  --set crds.install=false \
  --set rbac.create=true \
  --set webhook.enabled=false --set certManager.enabled=false \
  --set ui.api.enabled=false --set ui.web.enabled=false

# Step 2 (앱 팀): 오퍼레이터 워크로드를 후속으로 기동.
#   동일 릴리스 이름을 사용하면 RBAC가 유지되고 ServiceAccount 주체가 일치함.
helm upgrade aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator \
  --set operator.enabled=true \
  --set crds.install=false
```

차트 소스에서 설치한다면 `values-cluster-admin.yaml` preset에 Step 1의 override가 모두 들어 있습니다.

```bash
helm install aerospike-ce-kubernetes-operator ./charts/aerospike-ce-kubernetes-operator \
  -f ./charts/aerospike-ce-kubernetes-operator/values-cluster-admin.yaml \
  -n aerospike-operator --create-namespace
```

Step 1은 manager + metrics `ClusterRole`/`ClusterRoleBinding`**만** 렌더링합니다. 이 바인딩이 참조하는 `ServiceAccount`(및 네임스페이스 범위 리더 선출 `Role`/`RoleBinding`)는 Step 2에서 오퍼레이터 워크로드와 함께 생성됩니다 — 아직 생성되지 않은 `ServiceAccount`를 참조하는 `ClusterRoleBinding`은 Kubernetes에서 유효하며, 두 단계는 서로 겹치지 않는(disjoint) 리소스를 소유합니다. 이를 **별도** 릴리스(다른 팀 소유)로 유지하려면 `fullnameOverride`로 리소스 이름을 맞추고 오퍼레이터 릴리스에 `rbac.create=false`를 설정하세요.

</TabItem>
<TabItem value="helm-gitops" label="Helm + GitOps (ArgoCD / Flux)">

GitOps 환경에서는 CRD와 오퍼레이터 라이프사이클을 독립적으로 관리하도록 CRD를 따로 설치합니다.

**Step 1: 클러스터당 한 번 CRD 설치**

```bash
helm install aerospike-ce-kubernetes-operator-crds oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator-crds \
  --version 1.10.3
```

CRD는 `helm.sh/resource-policy: keep` 어노테이션이 있어 `helm uninstall` 시에도 **삭제되지 않아** 클러스터 데이터를 보호합니다.

**Step 2: 오퍼레이터 설치 (CRD 설치 건너뛰기)**

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --version 1.10.3 \
  --set crds.install=false \
  -n aerospike-operator --create-namespace
```

**ArgoCD 예제 (sync-wave)**

```yaml
# Application 1: CRDs — 자동 정리하지 않음
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: aerospike-ce-kubernetes-operator-crds
  annotations:
    argocd.argoproj.io/sync-options: Replace=true
spec:
  source:
    repoURL: ghcr.io/aerospike-ce-ecosystem/charts
    chart: aerospike-ce-kubernetes-operator-crds
    targetRevision: "1.10.3"
  syncPolicy:
    automated:
      prune: false
      selfHeal: true
---
# Application 2: Operator
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: aerospike-ce-kubernetes-operator
spec:
  source:
    repoURL: ghcr.io/aerospike-ce-ecosystem/charts
    chart: aerospike-ce-kubernetes-operator
    targetRevision: "1.10.3"
    helm:
      values: |
        crds:
          install: false
  destination:
    namespace: aerospike-operator
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
    syncOptions:
      - CreateNamespace=true
```

**Flux 예제**

```yaml
# HelmRepository (OCI) — 두 HelmRelease가 공유
apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
metadata:
  name: aerospike-ce-kubernetes-operator
  namespace: flux-system
spec:
  type: oci
  url: oci://ghcr.io/aerospike-ce-ecosystem/charts
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: aerospike-ce-kubernetes-operator-crds
  namespace: flux-system
spec:
  chart:
    spec:
      chart: aerospike-ce-kubernetes-operator-crds
      version: "1.10.3"
      sourceRef:
        kind: HelmRepository
        name: aerospike-ce-kubernetes-operator
  install:
    crds: CreateReplace
  upgrade:
    crds: CreateReplace
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: aerospike-ce-kubernetes-operator
  namespace: flux-system
spec:
  dependsOn:
    - name: aerospike-ce-kubernetes-operator-crds
  targetNamespace: aerospike-operator
  chart:
    spec:
      chart: aerospike-ce-kubernetes-operator
      version: "1.10.3"
      sourceRef:
        kind: HelmRepository
        name: aerospike-ce-kubernetes-operator
  values:
    crds:
      install: false
```

</TabItem>
<TabItem value="local-build" label="로컬 빌드">

소스에서 직접 빌드하는 개발자/기여자용 방법입니다.

```bash
git clone https://github.com/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator.git
cd aerospike-ce-kubernetes-operator

# 오퍼레이터 이미지를 빌드하고 레지스트리에 푸시
make docker-build docker-push IMG=<your-registry>/aerospike-ce-kubernetes-operator:latest

# CRD 설치
make install

# 오퍼레이터 배포
make deploy IMG=<your-registry>/aerospike-ce-kubernetes-operator:latest
```

</TabItem>
</Tabs>

## 모니터링 (선택사항)

Helm 차트에는 Prometheus Operator 모니터링 리소스가 내장되어 있습니다.
모든 모니터링 기능은 **기본적으로 비활성화**되어 있으며, [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator) (일반적으로 [kube-prometheus-stack](https://github.com/prometheus-community/helm-charts/tree/main/charts/kube-prometheus-stack)을 통해 설치)가 필요합니다.

### ServiceMonitor

Prometheus가 오퍼레이터 메트릭을 자동으로 스크레이핑하도록 `ServiceMonitor` 리소스를 생성합니다.

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.additionalLabels.release=prometheus
```

:::tip
`additionalLabels.release=prometheus` label은 Prometheus Operator의 `serviceMonitorSelector`와 일치해야 합니다. 다음 명령으로 값을 확인합니다.
```bash
kubectl get prometheus -A -o jsonpath='{.items[*].spec.serviceMonitorSelector}'
```
:::

| 파라미터 | 기본값 | 설명 |
|---------|--------|------|
| `serviceMonitor.enabled` | `false` | ServiceMonitor 리소스 생성 여부 |
| `serviceMonitor.interval` | — | 스크레이핑 주기 (예: `"30s"`) |
| `serviceMonitor.scrapeTimeout` | — | 스크레이핑 타임아웃 (예: `"10s"`) |
| `serviceMonitor.additionalLabels` | `{}` | Prometheus selector 매칭을 위한 추가 라벨 |

### PrometheusRule

오퍼레이터를 위한 내장 알림 규칙이 포함된 `PrometheusRule` 리소스를 생성합니다.

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set serviceMonitor.enabled=true \
  --set prometheusRule.enabled=true
```

**내장 알림 규칙** (`prometheusRule.rules`가 비어있을 때 사용):

| 알림 | 조건 | 심각도 |
|------|------|--------|
| `AerospikeOperatorDown` | 오퍼레이터 5분간 접근 불가 | critical |
| `AerospikeOperatorReconcileErrors` | Reconcile 오류율 > 0 (15분간) | warning |
| `AerospikeOperatorSlowReconcile` | p99 reconcile 시간 > 60초 (10분간) | warning |
| `AerospikeOperatorWorkQueueDepth` | 큐 깊이 > 10 (10분간) | warning |
| `AerospikeOperatorHighMemory` | 메모리 > 256Mi (10분간) | warning |
| `AerospikeOperatorPodRestarts` | 1시간 내 3회 이상 재시작 | warning |

내장 기본값 대신 **커스텀 규칙**을 사용하려면 `values.yaml` 파일을 사용합니다:

```yaml
prometheusRule:
  enabled: true
  rules:
    - alert: CustomAerospikeAlert
      expr: up{job="aerospike"} == 0
      for: 5m
      labels:
        severity: critical
      annotations:
        summary: "Custom Aerospike alert"
```

| 파라미터 | 기본값 | 설명 |
|---------|--------|------|
| `prometheusRule.enabled` | `false` | PrometheusRule 리소스 생성 여부 |
| `prometheusRule.additionalLabels` | `{}` | Prometheus selector 매칭을 위한 추가 라벨 |
| `prometheusRule.rules` | `[]` | 커스텀 규칙; 비어있으면 내장 기본값 사용 |

### Grafana 대시보드

사전 구성된 Grafana 대시보드가 포함된 `ConfigMap`을 생성합니다. [Grafana sidecar](https://github.com/grafana/helm-charts/tree/main/charts/grafana#sidecar-for-dashboards)가 자동 발견을 위해 설정되어 있어야 합니다.

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set grafanaDashboard.enabled=true
```

대시보드에 포함된 패널: **Reconcile Rate**, **Reconcile Errors**, **Reconcile Duration (p99/p50)**, **Work Queue Depth**, **Operator Memory Usage**, **Operator CPU Usage**.

| 파라미터 | 기본값 | 설명 |
|---------|--------|------|
| `grafanaDashboard.enabled` | `false` | 대시보드 ConfigMap 생성 여부 |
| `grafanaDashboard.sidecarLabel` | `grafana_dashboard` | Grafana sidecar 발견을 위한 라벨 키 |
| `grafanaDashboard.sidecarLabelValue` | `"1"` | Grafana sidecar 발견을 위한 라벨 값 |
| `grafanaDashboard.folder` | `""` | 대시보드 정리를 위한 Grafana 폴더 이름 |

### Grafana 설치 및 대시보드 자동 발견 설정

Grafana를 sidecar 활성화 상태로 설치하고 port-forward를 통해 오퍼레이터 대시보드를 확인하는 단계별 가이드입니다.

**1. Grafana Helm 리포지토리 추가:**

```bash
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update
```

**2. sidecar를 활성화하여 Grafana 설치:**

```bash
helm install grafana grafana/grafana \
  -n monitoring --create-namespace \
  --set sidecar.dashboards.enabled=true \
  --set sidecar.dashboards.label=grafana_dashboard \
  --set sidecar.dashboards.labelValue="1" \
  --set sidecar.dashboards.searchNamespace=ALL \
  --set sidecar.datasources.enabled=true
```

:::info
`sidecar.dashboards.searchNamespace=ALL`을 설정하면 sidecar가 `aerospike-operator`를 포함한 **모든 네임스페이스**에서 dashboard ConfigMap을 찾습니다. 설정하지 않으면 sidecar는 자신이 속한 네임스페이스만 감시합니다.
:::

**3. 대시보드를 활성화하여 오퍼레이터 설치 (또는 업그레이드):**

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set grafanaDashboard.enabled=true
```

**4. Grafana admin 비밀번호 확인:**

```bash
kubectl -n monitoring get secret grafana -o jsonpath="{.data.admin-password}" | base64 -d; echo
```

**5. Grafana에 접근하기 위한 port-forward 실행:**

```bash
kubectl -n monitoring port-forward svc/grafana 3000:80
```

**6.** 브라우저에서 [http://localhost:3000](http://localhost:3000)을 열고 다음으로 로그인합니다:
- **사용자명:** `admin`
- **비밀번호:** 4단계에서 확인한 값

**"Aerospike CE Operator"** 대시보드가 **Dashboards** 아래에 자동으로 나타납니다. `grafanaDashboard.folder`를 설정한 경우 지정된 폴더 아래에 정리됩니다.

:::tip
대시보드가 나타나지 않는 경우, ConfigMap이 올바른 라벨로 생성되었는지 확인하세요:
```bash
kubectl -n aerospike-operator get configmap -l grafana_dashboard=1
```
:::

### 전체 모니터링 스택 예제

모든 모니터링 기능을 한 번에 활성화:

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.additionalLabels.release=prometheus \
  --set prometheusRule.enabled=true \
  --set grafanaDashboard.enabled=true \
  --set grafanaDashboard.folder=Aerospike
```

## Cluster Manager UI (선택사항)

[Aerospike Cluster Manager](https://github.com/aerospike-ce-ecosystem/aerospike-cluster-manager)는 Aerospike CE를 관리하는 웹 UI입니다. 레코드 탐색, 쿼리 빌더, 인덱스 관리, K8s 클러스터 라이프사이클 작업을 지원합니다.

### 오퍼레이터와 클러스터 매니저의 관계

Aerospike CE Kubernetes Operator와 Aerospike Cluster Manager는 함께 동작하는 두 개의 별도 컴포넌트입니다:

- **오퍼레이터** (`aerospike-ce-kubernetes-operator`): `AerospikeCluster`와 `AerospikeClusterTemplate` custom resource를 감시하고 desired state를 조정하는 Kubernetes controller입니다. StatefulSet, Service, ConfigMap을 만들고 rolling update, scaling, 랙 관리를 처리합니다.
- **클러스터 매니저** (`aerospike-cluster-manager`): Aerospike 클러스터(데이터 작업, 모니터링)와 Kubernetes API(CRD를 통한 클러스터 라이프사이클)를 모두 다루는 GUI를 제공하는 웹 애플리케이션(Next.js 프론트엔드 + FastAPI 백엔드)입니다.

Helm 차트 0.4.0+에서는 클러스터 매니저가 기본으로 오퍼레이터와 동일한 네임스페이스에 별도 Deployment로 배포됩니다 (`ui.api.enabled` / `ui.web.enabled`로 개별 토글 가능). 클러스터 매니저는 다음과 통신합니다:
1. **Aerospike 클러스터** — 데이터 작업(레코드 탐색, AQL, 인덱스 관리, UDF 관리)을 위해 Aerospike 와이어 프로토콜로 직접 통신
2. **Kubernetes API** — 클러스터 라이프사이클 작업(`AerospikeCluster` CR의 생성, 스케일, 편집, 삭제)을 위해 통신하며, 이후 오퍼레이터가 조정

오퍼레이터는 Cluster Manager와 독립적으로 동작합니다. `kubectl`과 YAML manifest만으로도 클러스터를 모두 관리할 수 있으며, Cluster Manager는 같은 작업을 위한 GUI를 제공합니다.

### UI 활성화

UI는 기본 활성화이므로 별도 플래그 없이 설치하면 함께 올라옵니다:

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace
```

UI를 끄려면 `--set ui.api.enabled=false --set ui.web.enabled=false`.

### Port-Forward로 접근

웹 frontend Service는 3100 포트를 노출합니다.

```bash
kubectl -n aerospike-operator port-forward svc/aerospike-ce-kubernetes-operator-ui-web 3100:3100
```

브라우저에서 [http://localhost:3100](http://localhost:3100)을 엽니다.

:::tip
웹 서비스 이름은 `<release>-aerospike-ce-kubernetes-operator-ui-web` 형식입니다. 릴리스 이름을 커스텀으로 지정한 경우 아래와 같이 조정하세요:
```bash
kubectl -n aerospike-operator port-forward svc/<release>-aerospike-ce-kubernetes-operator-ui-web 3100:3100
```
:::

### Ingress로 접근

외부에서 지속적으로 접근하려면 Ingress를 활성화합니다:

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set ui.ingress.enabled=true \
  --set ui.ingress.className=nginx \
  --set "ui.ingress.hosts[0].host=aerospike-admin.example.com" \
  --set "ui.ingress.hosts[0].paths[0].path=/" \
  --set "ui.ingress.hosts[0].paths[0].pathType=Prefix"
```

### 주요 파라미터

| 파라미터 | 기본값 | 설명 |
|---------|--------|------|
| `ui.api.enabled` | `true` | Cluster Manager API (FastAPI) 컴포넌트 배포. `ui.web.enabled=false`와 함께 false로 설정하면 UI 전체 비활성. |
| `ui.web.enabled` | `true` | Cluster Manager web (Next.js) 컴포넌트 배포. `ui.api.enabled=false`와 함께 false로 설정하면 UI 전체 비활성. |
| `ui.replicaCount` | `1` | UI 레플리카 수 |
| `ui.api.service.port` | `80` | API 서비스 포트 (컨테이너 포트 8000으로 전달) |
| `ui.web.service.port` | `3100` | Web 서비스 포트 (브라우저 접근 / Ingress 대상) |
| `ui.ingress.enabled` | `false` | 외부 접근을 위한 Ingress 생성 |
| `ui.database.type` | `sqlite` | 데이터베이스 백엔드: `sqlite`(내장, 기본값) 또는 `postgresql`(외부) |
| `ui.database.sqlite.persistence.enabled` | `true` | SQLite 데이터베이스 파일을 PVC에 영속 저장 |
| `ui.database.sqlite.persistence.size` | `1Gi` | SQLite PVC 스토리지 크기 |
| `ui.database.postgresql.databaseUrl` | `""` | 외부 PostgreSQL 연결 URL (`type=postgresql`일 때) |
| `ui.database.postgresql.existingSecret` | `""` | `DATABASE_URL` 키를 포함하는 기존 Secret (`databaseUrl` 대안) |
| `ui.k8s.enabled` | `true` | K8s 클러스터 관리 기능 활성화 (UI에서 클러스터 생성) |

### 외부 PostgreSQL 사용

기본 백엔드는 내장 SQLite입니다. 임베디드 PostgreSQL 사이드카는 제거되었으며, PostgreSQL은 외부 인스턴스로만 사용할 수 있습니다. 직접 운영하는 외부 PostgreSQL에 연결하려면:

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set ui.database.type=postgresql \
  --set ui.database.postgresql.databaseUrl="postgresql://user:pass@db-host:5432/aerospike_manager"
```

:::warning
구버전의 `ui.postgresql.*` / `ui.persistence.*` 키는 이제 설치 시 마이그레이션 안내 메시지와 함께 실패합니다. `ui.postgresql.enabled: true` → `ui.database.type: postgresql` + `ui.database.postgresql.databaseUrl`, `ui.postgresql.enabled: false` → `ui.database.type: sqlite`, `ui.persistence.*` → `ui.database.sqlite.persistence.*`로 매핑하세요.
:::

## 설치 확인

오퍼레이터 파드가 실행 중인지 확인:

```bash
kubectl -n aerospike-operator get pods
```

예상 출력:

```
NAME                                                READY   STATUS    RESTARTS   AGE
aerospike-operator-controller-manager-xxxxx-yyyyy   1/1     Running   0          30s
```

CRD가 등록되었는지 확인:

```bash
kubectl get crd aerospikeclusters.acko.io
```

## 빠른 시작: 전체 설치 스크립트

Kind 클러스터에서 cert-manager, Prometheus, 오퍼레이터(모니터링 전체 활성화), Grafana, 샘플 Aerospike 클러스터, 검증까지 한 번에 복사-붙여넣기로 설정하는 스크립트입니다.

:::info 사전 준비
- [Kind](https://kind.sigs.k8s.io/) 설치
- [Helm](https://helm.sh/) v3 설치
- [kubectl](https://kubernetes.io/docs/tasks/tools/) 설치

먼저 Kind 클러스터를 생성합니다:
```bash
kind create cluster --config kind-config.yaml --name kind
```
:::

```bash
# =============================================================================
# 1. cert-manager 설치
# =============================================================================
helm repo add jetstack https://charts.jetstack.io
helm repo update jetstack
helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true \
  --wait

# =============================================================================
# 2. Prometheus Operator 설치 (kube-prometheus-stack)
# =============================================================================
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update prometheus-community
helm install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  --create-namespace \
  --set grafana.enabled=false \
  --wait

# =============================================================================
# 3. Aerospike Operator 설치 (모니터링 전체 활성화)
# =============================================================================
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.additionalLabels.release=prometheus \
  --set prometheusRule.enabled=true \
  --set grafanaDashboard.enabled=true \
  --set grafanaDashboard.folder=Aerospike \
  --wait

# =============================================================================
# 4. Grafana 설치 (sidecar 대시보드 자동 발견 활성화)
# =============================================================================
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update grafana
helm install grafana grafana/grafana \
  --namespace monitoring \
  --set sidecar.dashboards.enabled=true \
  --set sidecar.dashboards.label=grafana_dashboard \
  --set sidecar.dashboards.labelValue="1" \
  --set sidecar.dashboards.searchNamespace=ALL \
  --set sidecar.datasources.enabled=true \
  --wait

# =============================================================================
# 5. Aerospike CE 클러스터 배포
# =============================================================================
kubectl create namespace aerospike --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f config/samples/acko_v1alpha1_aerospikecluster.yaml

echo "Aerospike 파드 준비 대기 중..."
kubectl -n aerospike wait --for=condition=Ready pod/aerospike-basic-0-0 --timeout=120s

# =============================================================================
# 6. 검증: Aerospike 파드에서 asinfo 실행
# =============================================================================
echo "=== Aerospike 클러스터 정보 ==="
kubectl -n aerospike exec -it aerospike-basic-0-0 -- asinfo -v status
kubectl -n aerospike exec -it aerospike-basic-0-0 -- asinfo -v build

# =============================================================================
# 7. Grafana port-forward (http://localhost:3000 에서 접속)
# =============================================================================
GRAFANA_PASSWORD=$(kubectl -n monitoring get secret grafana \
  -o jsonpath="{.data.admin-password}" | base64 -d)
echo ""
echo "=== Grafana ==="
echo "URL:      http://localhost:3000"
echo "사용자명: admin"
echo "비밀번호: ${GRAFANA_PASSWORD}"
echo ""
kubectl -n monitoring port-forward svc/grafana 3000:80
```

:::tip
각 Helm install에 `--wait` 옵션을 사용하여 이전 컴포넌트가 완전히 준비된 후 다음 단계가 시작됩니다. CI에서 마지막 `port-forward` 없이 실행하려면 마지막 명령을 제거하세요.
:::

## 제거

:::warning
오퍼레이터를 제거하기 전에 반드시 AerospikeCluster 리소스를 먼저 삭제하세요. 오퍼레이터를 먼저 제거하면 고아 상태의 StatefulSet과 PVC가 남게 됩니다.
:::

<Tabs groupId="install-method">
<TabItem value="helm-oci" label="Helm" default>

```bash
# 먼저 모든 Aerospike 클러스터를 삭제
kubectl delete asc --all --all-namespaces

# 오퍼레이터 제거
helm uninstall aerospike-ce-kubernetes-operator -n aerospike-operator

# (선택) 네임스페이스 삭제
kubectl delete namespace aerospike-operator
```

</TabItem>
<TabItem value="local-build" label="로컬 빌드">

```bash
# 먼저 모든 Aerospike 클러스터를 삭제
kubectl delete asc --all --all-namespaces

# 오퍼레이터 제거
make undeploy

# CRD 제거
make uninstall
```

</TabItem>
</Tabs>
