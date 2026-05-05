---
sidebar_position: 1
slug: /
title: Quick Start
---

# Quick Start

Deploy an Aerospike CE cluster with the Cluster Manager UI on a local Kind cluster.

## Prerequisites

- [Kind](https://kind.sigs.k8s.io/) — local Kubernetes cluster
- [kubectl](https://kubernetes.io/docs/tasks/tools/) — Kubernetes CLI
- [Helm](https://helm.sh/) v3.x — package manager

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

## Step 1: Create a Kind Cluster

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

## Step 2: Install cert-manager

```bash
helm repo add jetstack https://charts.jetstack.io
helm repo update
helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager --create-namespace \
  --set crds.enabled=true \
  --wait
```

## Step 3: Install the Operator

```bash
helm install aerospike-ce-kubernetes-operator \
  oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --wait
```

Verify all pods are running:

```bash
kubectl -n aerospike-operator get pods
```

```
NAME                                                  READY   STATUS    AGE
aerospike-ce-kubernetes-operator-xxxxx-yyyyy           1/1     Running   60s
aerospike-ce-kubernetes-operator-ui-xxxxx-yyyyy        2/2     Running   60s
```

## Step 4: Deploy an Aerospike Cluster

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

Wait for the cluster to be ready:

```bash
kubectl -n aerospike wait --for=condition=Ready pod/aerospike-basic-0-0 --timeout=120s
kubectl -n aerospike get asc
```

```
NAME              RACKSIZE   HEALTH   PHASE       AGE   AVAILABLE
aerospike-basic   1          1/1      Completed   60s   True
```

## Step 5: Access the Cluster Manager UI

```bash
kubectl -n aerospike-operator port-forward svc/aerospike-ce-kubernetes-operator-ui 3000:3000
```

Open [http://localhost:3000](http://localhost:3000) in your browser. The UI provides a visual dashboard for managing clusters, browsing records, and running AQL queries.

:::tip
Keep this terminal open — port-forward runs in the foreground. Open a new terminal for the next step.
:::

## Step 6: Connect with AQL

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

## Clean Up

```bash
kind delete cluster --name aerospike
```

## Next Steps

- [Installation Guide](./getting-started/install) — production installation options, Helm values, GitOps setup
- [Create Cluster](./getting-started/create-cluster) — multi-node, persistent storage, rack-aware configurations
- [Cluster Manager UI](./getting-started/cluster-manager-ui) — full UI feature guide and Ingress configuration
- [Manage Cluster](./operations/manage-cluster) — scaling, rolling updates, configuration changes
- [API Reference](./reference/api-reference/aerospikecluster) — complete CRD specification
