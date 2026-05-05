---
sidebar_position: 1
title: 설치
---

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# 설치

이 가이드는 ACKO 오퍼레이터를 설치하는 두 가지 방법을 다룹니다.

## 사전 준비

- Kubernetes 클러스터 v1.26+
- 클러스터에 접근 가능하도록 설정된 kubectl
- [cert-manager](https://cert-manager.io/) 설치 완료 (웹훅 TLS에 필요)

### cert-manager

cert-manager는 웹훅 TLS에 필요합니다. 오퍼레이터 설치 전에 먼저 설치하세요:

```bash
helm repo add jetstack https://charts.jetstack.io
helm repo update
helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true
```

cert-manager 준비 대기:

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

기본값을 재정의할 수 있습니다:

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

</TabItem>
<TabItem value="helm-gitops" label="Helm + GitOps (ArgoCD / Flux)">

GitOps 환경에서는 오퍼레이터 라이프사이클과 독립적으로 관리할 수 있도록 CRD를 별도로 설치합니다.

**Step 1: 클러스터당 한 번 CRD 설치**

```bash
helm install aerospike-ce-kubernetes-operator-crds oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator-crds \
  --version 0.1.0
```

CRD는 `helm.sh/resource-policy: keep` 어노테이션이 있어 `helm uninstall` 시에도 **삭제되지 않아** 클러스터 데이터를 보호합니다.

**Step 2: 오퍼레이터 설치 (CRD 설치 건너뛰기)**

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  --version 0.1.0 \
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
    targetRevision: "0.1.0"
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
    targetRevision: "0.1.0"
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
      version: "0.1.0"
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
      version: "0.1.0"
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
`additionalLabels.release=prometheus` 라벨은 Prometheus Operator의 `serviceMonitorSelector`와 일치해야 합니다. 다음 명령으로 확인할 수 있습니다:
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
`sidecar.dashboards.searchNamespace=ALL`을 설정하면 sidecar가 `aerospike-operator` 네임스페이스를 포함한 **모든 네임스페이스**에서 대시보드 ConfigMap을 발견할 수 있습니다. 이 설정이 없으면 sidecar는 자신이 속한 네임스페이스만 감시합니다.
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

[Aerospike Cluster Manager](https://github.com/aerospike-ce-ecosystem/aerospike-cluster-manager)는 Aerospike CE 클러스터를 관리하기 위한 웹 기반 GUI입니다 -- 레코드 탐색, 쿼리 빌더, 인덱스 관리, K8s 클러스터 라이프사이클 등을 제공합니다.

### 오퍼레이터와 클러스터 매니저의 관계

Aerospike CE Kubernetes Operator와 Aerospike Cluster Manager는 함께 동작하는 두 개의 별도 컴포넌트입니다:

- **오퍼레이터** (`aerospike-ce-kubernetes-operator`): `AerospikeCluster`와 `AerospikeClusterTemplate` 커스텀 리소스를 감시하고 원하는 상태를 조정하는 Kubernetes 컨트롤러입니다 -- StatefulSet, Service, ConfigMap을 생성하고 롤링 업데이트, 스케일링, 랙 관리를 수행합니다.
- **클러스터 매니저** (`aerospike-cluster-manager`): Aerospike 클러스터(데이터 작업, 모니터링)와 Kubernetes API(CRD를 통한 클러스터 라이프사이클)를 모두 다루는 GUI를 제공하는 웹 애플리케이션(Next.js 프론트엔드 + FastAPI 백엔드)입니다.

Helm 차트 0.4.0+에서는 클러스터 매니저가 기본으로 오퍼레이터와 동일한 네임스페이스에 별도 Deployment로 배포됩니다 (`ui.api.enabled` / `ui.web.enabled`로 개별 토글 가능). 클러스터 매니저는 다음과 통신합니다:
1. **Aerospike 클러스터** — 데이터 작업(레코드 탐색, AQL, 인덱스 관리, UDF 관리)을 위해 Aerospike 와이어 프로토콜로 직접 통신
2. **Kubernetes API** — 클러스터 라이프사이클 작업(`AerospikeCluster` CR의 생성, 스케일, 편집, 삭제)을 위해 통신하며, 이후 오퍼레이터가 조정

오퍼레이터는 클러스터 매니저와 독립적으로 동작합니다 -- `kubectl`과 YAML 매니페스트만으로도 클러스터를 완전히 관리할 수 있습니다. 클러스터 매니저는 단순히 편리한 GUI 레이어를 제공합니다.

### UI 활성화

UI는 기본 활성화이므로 별도 플래그 없이 설치하면 함께 올라옵니다:

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace
```

UI를 끄려면 `--set ui.api.enabled=false --set ui.web.enabled=false`.

### Port-Forward로 접근

```bash
kubectl -n aerospike-operator port-forward svc/aerospike-operator-ui 3000:3000
```

브라우저에서 [http://localhost:3000](http://localhost:3000)을 열면 UI에 접근할 수 있습니다.

:::tip
서비스 이름은 `<release>-aerospike-operator-ui` 형식입니다. 릴리스 이름을 커스텀으로 지정한 경우 아래와 같이 조정하세요:
```bash
kubectl -n aerospike-operator port-forward svc/<release>-aerospike-operator-ui 3000:3000
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
| `ui.service.type` | `ClusterIP` | 서비스 타입 (`ClusterIP`, `NodePort`, `LoadBalancer`) |
| `ui.service.frontendPort` | `3000` | Frontend(Next.js) 포트 |
| `ui.service.backendPort` | `8000` | Backend(FastAPI) 포트 |
| `ui.ingress.enabled` | `false` | 외부 접근을 위한 Ingress 생성 |
| `ui.postgresql.enabled` | `true` | 임베디드 PostgreSQL 사이드카 배포 |
| `ui.persistence.enabled` | `true` | PostgreSQL 데이터용 PVC 활성화 |
| `ui.persistence.size` | `1Gi` | PVC 스토리지 크기 |
| `ui.k8s.enabled` | `true` | K8s 클러스터 관리 기능 활성화 (UI에서 클러스터 생성) |
| `ui.env.databaseUrl` | `""` | 외부 PostgreSQL URL (`postgresql.enabled=false`일 때 사용) |

### 외부 PostgreSQL 사용

임베디드 사이드카 대신 기존 PostgreSQL 인스턴스를 사용하려면:

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set ui.postgresql.enabled=false \
  --set ui.env.databaseUrl="postgresql://user:pass@db-host:5432/aerospike_manager"
```

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
