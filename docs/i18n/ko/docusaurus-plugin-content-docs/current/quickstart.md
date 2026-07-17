---
sidebar_position: 1
slug: /
title: 빠른 시작
---

# 빠른 시작

이 빠른 시작에서는 로컬 Kind 클러스터에 Aerospike CE와 Cluster Manager UI를 실행합니다.

## 사전 준비

- [Kind](https://kind.sigs.k8s.io/) — 로컬 Kubernetes 클러스터
- [kubectl](https://kubernetes.io/docs/tasks/tools/) — Kubernetes CLI
- [Helm](https://helm.sh/) v3.x — 패키지 매니저

```bash
brew install kind kubectl helm
```

<details>
<summary>Linux</summary>

```bash
# kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

# Helm
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# Kind
go install sigs.k8s.io/kind@latest
```

</details>

## Step 1: Kind 클러스터 생성

```bash
kind create cluster --name aerospike
```

```
Creating cluster "aerospike" ...
 ✓ Ensuring node image (kindest/node:v1.32.0) 🖼
 ✓ Preparing nodes 📦
 ✓ Writing configuration 📜
 ✓ Starting control-plane 🕹️
 ✓ Installing CNI 🔌
 ✓ Installing StorageClass 💾
Set kubectl context to "kind-aerospike"
```

## Step 2: cert-manager 설치

```bash
helm repo add jetstack https://charts.jetstack.io
helm repo update
helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager --create-namespace \
  --set crds.enabled=true \
  --wait
```

## Step 3: 오퍼레이터 설치

```bash
helm install aerospike-ce-kubernetes-operator \
  oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --wait
```

모든 파드가 실행 중인지 확인:

```bash
kubectl -n aerospike-operator get pods
```

```
NAME                                                  READY   STATUS    AGE
aerospike-ce-kubernetes-operator-xxxxx-yyyyy           1/1     Running   60s
aerospike-ce-kubernetes-operator-ui-xxxxx-yyyyy        2/2     Running   60s
```

## Step 4: Aerospike 클러스터 배포

```bash
kubectl create namespace aerospike

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

클러스터가 준비될 때까지 대기:

```bash
kubectl -n aerospike wait --for=condition=Ready pod/aerospike-basic-0-0 --timeout=120s
kubectl -n aerospike get asc
```

```
NAME              RACKSIZE   HEALTH   PHASE       AGE   AVAILABLE
aerospike-basic   1          1/1      Completed   60s   True
```

## Step 5: 클러스터 매니저 UI 접속

```bash
kubectl -n aerospike-operator port-forward svc/aerospike-ce-kubernetes-operator-ui-web 3100:3100
```

브라우저에서 [http://localhost:3100](http://localhost:3100)을 엽니다. UI에서 클러스터 관리, 레코드 탐색, AQL 쿼리 실행 등을 할 수 있습니다.

:::tip
이 터미널은 열어두세요 — port-forward는 포그라운드에서 실행됩니다. 다음 단계를 위해 새 터미널을 여세요.
:::

## Step 6: AQL로 접속

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

aql> INSERT INTO test.users (PK, name, age) VALUES ("user1", "Alice", 30)
OK, 1 record affected.

aql> SELECT * FROM test.users
+---------+-----+
| name    | age |
+---------+-----+
| "Alice" | 30  |
+---------+-----+

aql> EXIT
```

## 정리

```bash
kind delete cluster --name aerospike
```

## 다음 단계

- [설치 가이드](./getting-started/install) — 프로덕션 설치 옵션, Helm values, GitOps 설정
- [클러스터 생성](./getting-started/create-cluster) — 멀티 노드, 영구 스토리지, 랙 인식 구성
- [클러스터 매니저 UI](./getting-started/cluster-manager-ui) — UI 기능 가이드 및 Ingress 구성
- [클러스터 관리](./operations/manage-cluster) — 스케일링, 롤링 업데이트, 설정 변경
- [API 레퍼런스](./reference/api-reference/aerospikecluster) — 전체 CRD 스펙 문서
