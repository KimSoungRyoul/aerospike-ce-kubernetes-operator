# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Aerospike Community Edition Kubernetes Operator — manages Aerospike CE cluster lifecycle (deploy, scale, rolling update, config management) via a custom `AerospikeCluster` CRD. Built with kubebuilder v4.12, controller-runtime v0.23.1, Go 1.25.

## Build & Development Commands

```bash
make build              # Build manager binary (runs manifests, generate, fmt, vet first)
make manifests          # Generate CRD, RBAC, webhook YAML into config/
make generate           # Generate DeepCopy methods
make test               # Unit + envtest integration tests (excludes e2e)
make lint               # golangci-lint
make lint-fix           # golangci-lint with auto-fix
make docker-build       # Build container image (IMG=ghcr.io/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator:latest)
make install            # Install CRDs into current k8s cluster
make deploy             # Deploy operator to current k8s cluster
```

Local development (run controller on host):
```bash
make run                # Run controller locally against current k8s context
```

Run a single test:
```bash
go test ./internal/configgen/ -run TestGenerateConfig -v
```

Run tests without the full make chain:
```bash
KUBEBUILDER_ASSETS="$(bin/setup-envtest use -p path)" go test ./...
```

E2E tests (requires Kind cluster):
```bash
make setup-test-e2e     # Create Kind cluster + load operator image
make test-e2e           # Run e2e tests
make cleanup-test-e2e   # Tear down Kind cluster
```

Test framework: Ginkgo v2 + Gomega. E2E tests are in `test/e2e/`, helpers in `test/utils/`.

## Architecture

### CRD
- **Group/Version/Kind**: `acko.io/v1alpha1/AerospikeCluster`
- Short names: `asc`
- Types split across `api/v1alpha1/`: `aerospikecluster_types.go` (main spec/status), `types_storage.go`, `types_network.go`, `types_pod.go`, `types_rack.go`

### AerospikeConfigSpec Pattern
`AerospikeConfigSpec` wraps `map[string]interface{}` in a struct with custom JSON marshaling to work around controller-gen's lack of support for `map[string]interface{}` directly. Always access the map via `.Value`:
```go
config := cluster.Spec.AerospikeConfig.Value  // map[string]interface{}
```
The type has `+kubebuilder:object:generate=false` and provides manual DeepCopy.

### Controller (internal/controller/)
- **Reconciler pattern**: Single `AerospikeClusterReconciler` struct in `reconciler.go`
- **Rack-per-StatefulSet**: Each rack ID gets its own StatefulSet (`<name>-<rackID>`) and ConfigMap (`<name>-<rackID>-config`)
- **UpdateStrategy**: `OnDelete` — operator manages pod deletion/recreation for rolling restarts
- **Reconciliation flow**: Fetch CR → finalizer → paused check → headless service → per-rack ConfigMap + StatefulSet → cleanup removed racks → PDB → rolling restart → status update
- Split into files by concern: `reconciler_statefulset.go`, `reconciler_config.go`, `reconciler_services.go`, `reconciler_pdb.go`, `reconciler_restart.go`, `reconciler_status.go`, `reconciler_cleanup.go`, `reconciler_acl.go`

### Config Generation (internal/configgen/)
Converts unstructured `map[string]interface{}` to aerospike.conf text format. Handles special sections: `namespaces` (array→named blocks), `logging`, `security`, `network` (mesh seed injection).

### Webhooks (api/v1alpha1/aerospikecluster_webhook.go)
- **Defaulter**: Auto-sets cluster-name, network ports (3000/3001/3002), heartbeat mode (mesh), proto-fd-max (15000)
- **Validator**: CE constraints — size<=8, namespaces<=2, no `xdr`/`tls` sections, no enterprise images, admin user required when ACL enabled, unique rack IDs


## Aerospike Configuration Guide

Aerospike CE 8.1 파라미터, CRD 설정, Day-2 운영 가이드는 ecosystem plugin에서 제공:
- `acko-config-reference` — CE 8.1 파라미터, breaking changes, CRD 매핑, Webhook 검증
- `acko-deploy` — K8s 배포 가이드, CR 예제 (8개 시나리오)
- `acko-operations` — Day-2 운영, 트러블슈팅, 진단 커맨드

## Sample CRs

Located in `config/samples/`:
- `acko_v1alpha1_aerospikecluster.yaml` — Minimal single-node in-memory
- `aerospike-cluster-3node.yaml` — 3-node with PV storage
- `aerospike-cluster-multirack.yaml` — 6-node multi-rack with zone affinity
- `aerospike-cluster-acl.yaml` — 3-node with ACL (roles, users, K8s secrets)


## image registry
- https://hub.docker.com/_/aerospike
- `aerospike:ce-8.1.1.1`  (minimum supported: CE 8)


## create kind cluster

```bash
kind create cluster --config kind-config.yaml --name kind
```
