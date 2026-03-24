---
sidebar_position: 1
slug: /
title: Quick Start
---

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# Quick Start

Deploy an Aerospike CE cluster on Kubernetes in minutes.

## Prerequisites

- Kubernetes cluster v1.26+ (or [Kind](https://kind.sigs.k8s.io/) for local development)
- kubectl configured to access the cluster
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

## Step 1: Create a Kind Cluster

Skip this step if you already have a Kubernetes cluster.

```bash
kind create cluster --name aerospike
```

## Step 2: Install cert-manager (Optional)

Skip this step if you plan to use the bundled cert-manager option in Step 3.

```bash
helm repo add jetstack https://charts.jetstack.io
helm repo update
helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true
```

Verify cert-manager is running:

```bash
kubectl -n cert-manager wait --for=condition=Available deployment/cert-manager --timeout=60s
```

## Step 3: Install the Operator

```bash
# With bundled cert-manager (recommended if you skipped Step 2)
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set certManagerSubchart.enabled=true
```

Verify the operator is running:

```bash
kubectl -n aerospike-operator get pods
```

## Step 4: Deploy an Aerospike Cluster

```bash
kubectl create namespace aerospike
```

Apply a minimal single-node in-memory cluster:

<Tabs groupId="apply-method">
<TabItem value="file" label="Sample File" default>

```bash
kubectl apply -f config/samples/acko_v1alpha1_aerospikecluster.yaml
```

</TabItem>
<TabItem value="inline" label="Inline YAML">

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

## Step 5: Verify

```bash
# Check cluster status (Phase should be "Completed")
kubectl -n aerospike get asc

# Check pods
kubectl -n aerospike get pods
```

Expected output:

```
NAME                 RACKSIZE   HEALTH   PHASE       AGE
aerospike-basic   1          1/1      Completed   60s
```

## Step 6: Connect to Aerospike

Launch an `aerospike-tools` pod to interact with the cluster:

<Tabs groupId="aerospike-tool">
<TabItem value="aql" label="aql (Interactive)" default>

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
<TabItem value="asinfo" label="asinfo (Health Check)">

```bash
kubectl -n aerospike run asinfo-client --rm -it --restart=Never \
  --image=aerospike/aerospike-tools:latest \
  -- asinfo -h aerospike-basic -p 3000 -v status
```

```
ok
```

```bash
# Namespace statistics
kubectl -n aerospike run asinfo-client --rm -it --restart=Never \
  --image=aerospike/aerospike-tools:latest \
  -- asinfo -h aerospike-basic -p 3000 -v "namespace/test"
```

</TabItem>
</Tabs>

## Deploy with Cluster Manager UI (Optional)

Add `--set ui.enabled=true` to the Helm install command to deploy the web-based management UI alongside the operator:

```bash
helm install aerospike-ce-kubernetes-operator oci://ghcr.io/aerospike-ce-ecosystem/charts/aerospike-ce-kubernetes-operator \
  -n aerospike-operator --create-namespace \
  --set certManagerSubchart.enabled=true \
  --set ui.enabled=true
```

Access the UI via port-forward:

```bash
kubectl -n aerospike-operator port-forward svc/aerospike-ce-kubernetes-operator-ui 3000:3000
# Open http://localhost:3000
```

The UI provides a visual wizard for creating and managing Aerospike clusters, record browsing, AQL terminal, and more. See the [Cluster Manager UI](./guide/cluster-manager-ui) guide for details.

## Next Steps

- [Installation Guide](./guide/install) — detailed installation options (Helm, Kustomize)
- [Create Cluster](./guide/create-cluster) — sample configurations and CRD field reference
- [Manage Cluster](./guide/manage-cluster) — scaling, rolling updates, monitoring
- [Cluster Manager UI](./guide/cluster-manager-ui) — web-based GUI for browsing records, managing clusters, and executing AQL
- [API Reference](./api-reference/aerospikecluster) — full CRD type documentation
