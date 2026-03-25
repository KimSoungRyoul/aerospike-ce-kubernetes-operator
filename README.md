# Aerospike CE Kubernetes Operator (ACKO)

Kubernetes Operator for managing [Aerospike Community Edition](https://aerospike.com/) clusters. Built with [Kubebuilder](https://book.kubebuilder.io/) and [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime).

Deploy, scale, and perform rolling updates of Aerospike CE clusters via a custom `AerospikeCluster` CRD.

## Features

- Declarative cluster lifecycle management (create, scale, rolling update)
- Rack-aware deployment with zone/region affinity
- Persistent volume storage with cascade delete support
- Aerospike configuration management (auto-generates `aerospike.conf`)
- Access control (ACL) with Kubernetes Secrets integration
- Pod Disruption Budget for safe maintenance
- Mesh heartbeat auto-configuration
- Rack-node selection with zone/region affinity and rackLabel scheduling
- Optional web UI (Aerospike Cluster Manager) deployable via Helm (`ui.enabled=true`)

### CE Limitations

| Constraint | Limit |
|---|---|
| Max cluster size | 8 nodes |
| Max namespaces | 2 |
| XDR | Not supported |
| TLS | Not supported |
| Strong Consistency | Not supported |

## Terminology

| Term | Meaning in ACKO |
|------|----------------|
| **Rack** | Aerospike logical failure domain → one StatefulSet per rack |
| **Aerospike Node** | Single `asd` process; maps 1:1 to a K8s Pod |
| **Kubernetes Node** | Worker machine; distinct from Aerospike node |
| **Namespace** | Dual meaning: Aerospike data partition OR K8s namespace |
| **AerospikeCluster** | The CRD Kind for a cluster CR (`kubectl get asc`) |

→ Full glossary: [docs/content/guide/glossary.md](docs/content/guide/glossary.md)

## Quick Start

### Prerequisites

- Kubernetes cluster v1.26+ (or [Kind](https://kind.sigs.k8s.io/) for local development)
- kubectl configured to access the cluster
- [Helm](https://helm.sh/) v3.x

Install required tools (macOS):

```sh
brew install kind kubectl helm
```

### Step 1: Create a Kind Cluster (local development)

Skip this step if you already have a Kubernetes cluster.

```sh
kind create cluster --config kind-config.yaml
```

### Step 2: Install cert-manager

cert-manager is required for webhook TLS certificate management.

```sh
helm repo add jetstack https://charts.jetstack.io
helm repo update
helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager \
  --create-namespace \
  --set crds.enabled=true
```

Verify cert-manager is running:

```sh
kubectl -n cert-manager wait --for=condition=Available deployment/cert-manager --timeout=60s
```

### Step 3: Install the Operator

#### Option A: From OCI Registry (Recommended)

```sh
helm install aerospike-operator oci://ghcr.io/aerospike-ce-ecosystem/aerospike-operator \
  --version 0.1.0 \
  -n aerospike-operator --create-namespace
```

#### Option B: From Local Chart

```sh
helm install aerospike-operator ./charts/aerospike-operator \
  -n aerospike-operator --create-namespace
```

#### Option C: With Kustomize

```sh
# Build and push the operator image
make docker-build docker-push IMG=<your-registry>/aerospike-ce-kubernetes-operator:latest

# Install CRDs and deploy the operator
make install
make deploy IMG=<your-registry>/aerospike-ce-kubernetes-operator:latest
```

Verify the operator is running:

```sh
kubectl -n aerospike-operator get pods
```

### Step 4: Deploy an Aerospike Cluster

```sh
kubectl create namespace aerospike
kubectl apply -f config/samples/acko_v1alpha1_aerospikecluster.yaml
```

Or apply inline:

```yaml
apiVersion: acko.io/v1alpha1
kind: AerospikeCluster
metadata:
  name: aerospike-ce-basic
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
```

### Step 5: Verify

```sh
# Check cluster status (Phase should be "Completed")
kubectl -n aerospike get asc

# Check pods
kubectl -n aerospike get pods
```

### Step 6: Connect to Aerospike

Launch an `aerospike-tools` pod to interact with the cluster:

```sh
kubectl -n aerospike run aql-client --rm -it --restart=Never \
  --image=aerospike/aerospike-tools:latest \
  -- aql -h aerospike-ce-basic -p 3000
```

#### Show namespaces

```
aql> SHOW NAMESPACES
+--------+
| ns     |
+--------+
| "test" |
+--------+
```

#### Insert records

```
aql> INSERT INTO test.users (PK, name, age, email) VALUES ("user1", "Alice", 30, "alice@example.com")
OK, 1 record affected.

aql> INSERT INTO test.users (PK, name, age, email) VALUES ("user2", "Bob", 25, "bob@example.com")
OK, 1 record affected.

aql> INSERT INTO test.users (PK, name, age, email) VALUES ("user3", "Charlie", 35, "charlie@example.com")
OK, 1 record affected.
```

#### Read (SELECT)

```
aql> SELECT * FROM test.users WHERE PK = "user1"
+---------+-----+-------------------+
| name    | age | email             |
+---------+-----+-------------------+
| "Alice" | 30  | "alice@example.com" |
+---------+-----+-------------------+

aql> SELECT * FROM test.users
+-----------+-----+---------------------+
| name      | age | email               |
+-----------+-----+---------------------+
| "Alice"   | 30  | "alice@example.com"   |
| "Bob"     | 25  | "bob@example.com"     |
| "Charlie" | 35  | "charlie@example.com" |
+-----------+-----+---------------------+
```

#### Update a record

```
aql> INSERT INTO test.users (PK, name, age, email) VALUES ("user1", "Alice", 31, "alice-new@example.com")
OK, 1 record affected.

aql> SELECT * FROM test.users WHERE PK = "user1"
+---------+-----+-----------------------+
| name    | age | email                 |
+---------+-----+-----------------------+
| "Alice" | 31  | "alice-new@example.com" |
+---------+-----+-----------------------+
```

#### Delete a record

```
aql> DELETE FROM test.users WHERE PK = "user2"
OK, 1 record affected.
```

#### Check cluster info with asinfo

```sh
kubectl -n aerospike run asinfo-client --rm -it --restart=Never \
  --image=aerospike/aerospike-tools:latest \
  -- asinfo -h aerospike-ce-basic -p 3000 -v status
```

```
ok
```

```sh
# Namespace statistics
kubectl -n aerospike run asinfo-client --rm -it --restart=Never \
  --image=aerospike/aerospike-tools:latest \
  -- asinfo -h aerospike-ce-basic -p 3000 -v "namespace/test"
```

```
objects=3;sub_objects=0;...;memory_used_bytes=384;...
```

## More Examples

| Sample | Description |
|---|---|
| [`acko_v1alpha1_aerospikecluster.yaml`](config/samples/acko_v1alpha1_aerospikecluster.yaml) | Minimal single-node in-memory |
| [`aerospike-cluster-3node.yaml`](config/samples/aerospike-cluster-3node.yaml) | 3-node with persistent volume storage |
| [`aerospike-cluster-multirack.yaml`](config/samples/aerospike-cluster-multirack.yaml) | 6-node multi-rack with zone affinity |
| [`aerospike-cluster-acl.yaml`](config/samples/aerospike-cluster-acl.yaml) | 3-node with ACL (roles, users, K8s secrets) |
| [`aerospike-cluster-monitoring.yaml`](config/samples/aerospike-cluster-monitoring.yaml) | Prometheus monitoring with exporter sidecar |
| [`aerospike-cluster-storage-advanced.yaml`](config/samples/aerospike-cluster-storage-advanced.yaml) | Advanced storage policies and multiple volume types |
| [`aerospike-cluster-with-template.yaml`](config/samples/aerospike-cluster-with-template.yaml) | Using AerospikeClusterTemplate for config profiles |
| [`acko_v1alpha1_template_dev.yaml`](config/samples/acko_v1alpha1_template_dev.yaml) | Dev template (minimal resources, no anti-affinity) |
| [`acko_v1alpha1_template_stage.yaml`](config/samples/acko_v1alpha1_template_stage.yaml) | Stage template (moderate resources, soft anti-affinity) |
| [`acko_v1alpha1_template_prod.yaml`](config/samples/acko_v1alpha1_template_prod.yaml) | Prod template (full resources, hard anti-affinity, local PV) |

## Uninstall

```sh
# Delete the Aerospike cluster
kubectl -n aerospike delete asc aerospike-ce-basic

# Uninstall the operator (Helm)
helm uninstall aerospike-operator -n aerospike-operator

# Or uninstall the operator (Kustomize)
make undeploy
make uninstall
```

## Codebase Stats

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 Language              Files        Lines         Code     Comments       Blanks
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 Dockerfile                2           38           23            9            6
 Go                       80        18752        14554         1746         2452
 Makefile                  1          291          187           49           55
 Shell                     2          144          102           18           24
 TOML                      1           10            3            5            2
 YAML                     29         1057          851          137           69
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 Total                   115        20292        15720         1964         2608
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

> Measured with [tokei](https://github.com/XAMPPRocky/tokei) (`tokei -C`). Auto-generated code, Helm chart templates, and CRD/RBAC manifests are excluded via `.tokeignore`. JavaScript, JSON, CSS, and Markdown are excluded via `tokei.toml`.

## Development

```sh
make build          # Build manager binary
make test           # Run unit + integration tests
make lint           # Run golangci-lint
make run            # Run controller locally against current kubeconfig
```

### Local deployment with Kind

로컬 빌드 후 Kind 클러스터에 배포할 때:

```sh
# 1. Kind 클러스터 생성
make setup-test-e2e

# 2. 이미지 로컬 빌드
make docker-build

# 3. Kind 클러스터에 이미지 로드
kind load docker-image ghcr.io/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator:latest \
  --name aerospike-ce-kubernetes-operator-test-e2e

# 4. CRD 설치 및 오퍼레이터 배포
make install
make deploy
```

> `config/manager/manager.yaml`에 `imagePullPolicy: IfNotPresent`가 설정되어 있어, Kind에 로드된 로컬 이미지를 그대로 사용합니다.

## Claude Code

이 프로젝트는 [Claude Code](https://docs.anthropic.com/en/docs/claude-code) 자동화가 설정되어 있습니다.

### Ecosystem Plugin 설치

[aerospike-ce-ecosystem-plugins](https://github.com/aerospike-ce-ecosystem/aerospike-ce-ecosystem-plugins)를 설치하면 Aerospike CE 8.1 설정 레퍼런스, K8s 배포 가이드, Day-2 운영 가이드 등 ecosystem 전체 스킬을 사용할 수 있습니다.

```bash
claude plugin marketplace add aerospike-ce-ecosystem/aerospike-ce-ecosystem-plugins
claude plugin install aerospike-ce-ecosystem
```

## TODO

- [ ] Register OCI repository on [Artifact Hub](https://artifacthub.io/) — Add repository with Kind: **OCI**, URL: `oci://ghcr.io/aerospike-ce-ecosystem/aerospike-operator`

## License

Copyright 2026. Licensed under the Apache License, Version 2.0.
