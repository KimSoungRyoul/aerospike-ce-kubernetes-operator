---
sidebar_position: 1
slug: /
title: 빠른 시작
---

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# 빠른 시작

Kubernetes에 Aerospike CE 클러스터를 몇 분 만에 배포합니다.

## 사전 준비

- Kubernetes 클러스터 v1.26+ (또는 로컬 개발용 [Kind](https://kind.sigs.k8s.io/))
- 클러스터에 접근 가능하도록 설정된 kubectl
- [Helm](https://helm.sh/) v3.x

<Tabs groupId="os">
<TabItem value="macos" label="macOS" default>

```bash
brew install kind kubectl helm
```

</TabItem>
<TabItem value="linux" label="Linux">

```bash
# kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

# Helm
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# Kind
go install sigs.k8s.io/kind@latest
```

</TabItem>
</Tabs>

## Step 1: Kind 클러스터 생성

이미 Kubernetes 클러스터가 있다면 이 단계를 건너뛰세요.

```bash
kind create cluster --name aerospike
```

## Step 2: cert-manager 설치 (선택사항)

Step 3에서 번들 cert-manager 옵션을 사용할 예정이라면 이 단계를 건너뛰세요.

```bash
helm repo add jetstack https://charts.jetstack.io
helm repo update
helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true
```

cert-manager가 실행 중인지 확인:

```bash
kubectl -n cert-manager wait --for=condition=Available deployment/cert-manager --timeout=60s
```

## Step 3: 오퍼레이터 설치

```bash
# cert-manager 번들 설치 포함 (Step 2를 건너뛴 경우 권장)
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set certManagerSubchart.enabled=true
```

오퍼레이터가 실행 중인지 확인:

```bash
kubectl -n aerospike-operator get pods
```

## Step 4: Aerospike 클러스터 배포

```bash
kubectl create namespace aerospike
```

최소 단일 노드 인메모리 클러스터를 배포합니다:

<Tabs groupId="apply-method">
<TabItem value="file" label="샘플 파일" default>

```bash
kubectl apply -f config/samples/acko_v1alpha1_aerospikecluster.yaml
```

</TabItem>
<TabItem value="inline" label="인라인 YAML">

```bash
kubectl apply -f - <<'EOF'
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-basic
  namespace: aerospike
spec:
  size: 1
  image: aerospike:ce-8.1.1.1
  aerospikeConfig:
    namespaces:
      - name: test
        replication-factor: 1
        storage-engine:
          type: memory
          data-size: 1073741824
EOF
```

</TabItem>
</Tabs>

## Step 5: 확인

```bash
# 클러스터 상태 확인 (Phase가 "Completed"여야 함)
kubectl -n aerospike get asc

# 파드 확인
kubectl -n aerospike get pods
```

예상 출력:

```
NAME                 SIZE   PHASE       AGE
aerospike-basic   1      Completed   60s
```

## Step 6: Aerospike 접속

`aerospike-tools` 파드를 실행하여 클러스터와 상호작용합니다:

<Tabs groupId="aerospike-tool">
<TabItem value="aql" label="aql (인터랙티브)" default>

```bash
kubectl -n aerospike run aql-client --rm -it --restart=Never \
  --image=aerospike/aerospike-tools:latest \
  -- aql -h aerospike-basic -p 3000
```

```
aql> SHOW NAMESPACES
+--------+
| ns     |
+--------+
| "test" |
+--------+

aql> INSERT INTO test.users (PK, name, age, email) VALUES ("user1", "Alice", 30, "alice@example.com")
OK, 1 record affected.

aql> SELECT * FROM test.users
+---------+-----+---------------------+
| name    | age | email               |
+---------+-----+---------------------+
| "Alice" | 30  | "alice@example.com" |
+---------+-----+---------------------+
```

</TabItem>
<TabItem value="asinfo" label="asinfo (상태 확인)">

```bash
kubectl -n aerospike run asinfo-client --rm -it --restart=Never \
  --image=aerospike/aerospike-tools:latest \
  -- asinfo -h aerospike-basic -p 3000 -v status
```

```
ok
```

```bash
# 네임스페이스 통계
kubectl -n aerospike run asinfo-client --rm -it --restart=Never \
  --image=aerospike/aerospike-tools:latest \
  -- asinfo -h aerospike-basic -p 3000 -v "namespace/test"
```

</TabItem>
</Tabs>

## 클러스터 매니저 UI와 함께 배포 (선택사항)

Helm 설치 명령에 `--set ui.enabled=true`를 추가하면 오퍼레이터와 함께 웹 기반 관리 UI를 배포할 수 있습니다:

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set certManagerSubchart.enabled=true \
  --set ui.enabled=true
```

포트 포워딩으로 UI에 접근:

```bash
kubectl -n aerospike-operator port-forward svc/aerospike-ce-kubernetes-operator-ui 3000:3000
# http://localhost:3000 접속
```

UI는 Aerospike 클러스터 생성/관리를 위한 시각적 마법사, 레코드 브라우저, AQL 터미널 등을 제공합니다. 자세한 내용은 [클러스터 매니저 UI](./guide/cluster-manager-ui) 가이드를 참조하세요.

## 다음 단계

- [설치 가이드](./guide/install) — 상세한 설치 방법 (Helm, Kustomize)
- [클러스터 생성](./guide/create-cluster) — 샘플 설정 및 CRD 필드 참조
- [클러스터 관리](./guide/manage-cluster) — 스케일링, 롤링 업데이트, 모니터링
- [클러스터 매니저 UI](./guide/cluster-manager-ui) — 웹 기반 GUI로 레코드 탐색, 클러스터 관리, AQL 실행
- [API 레퍼런스](./api-reference/aerospikecluster) — 전체 CRD 타입 문서
