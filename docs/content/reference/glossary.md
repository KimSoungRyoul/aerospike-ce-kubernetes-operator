---
sidebar_position: 2
title: Glossary
---

# Terminology Glossary

Terms that have specific meanings in Aerospike, Kubernetes, and ACKO — or that are easily confused across these layers.

## Operator & CRD Concepts

| Term | Definition |
|------|-----------|
| **ACKO** | **A**eropike **C**ommunity Edition **K**ubernetes **O**perator. Manages `AerospikeCluster` and `AerospikeClusterTemplate` resources via the `acko.io` API group. |
| **AerospikeCluster** | The primary Custom Resource (CRD Kind) representing an Aerospike CE cluster deployment. `apiVersion: acko.io/v1alpha1`. Short name: `asc`. |
| **AerospikeClusterTemplate** | A CRD providing reusable configuration profiles for clusters. Referenced by `spec.templateRef`. Short name: `asct`. |
| **Rack** | A logical failure domain within an Aerospike cluster. In ACKO, each rack maps to one StatefulSet and one ConfigMap (`<clusterName>-<rackID>` pattern). Rack IDs are user-assigned positive integers; ID 0 is reserved internally. |
| **CR / Custom Resource** | An instance of a CRD. For example, a specific `AerospikeCluster` object in a namespace is a CR. |

## Aerospike vs. Kubernetes Term Disambiguation

| Term | In Aerospike | In Kubernetes |
|------|-------------|---------------|
| **Node** | A single Aerospike server process (`asd` daemon). In ACKO, one Aerospike node runs per Pod (1:1 mapping). | A worker machine (physical or virtual VM) in the K8s cluster. |
| **Namespace** | A data partition inside Aerospike — similar to a database. CE supports up to 2. Configured in `spec.aerospikeConfig.namespaces`. | An isolation boundary for K8s resources. The namespace where the CR lives. |
| **Cluster** | The set of Aerospike nodes that form a single distributed database. Represented by one `AerospikeCluster` CR. | The entire Kubernetes system (control plane + worker nodes). |

## Pod & Restart Concepts

| Term | Definition |
|------|-----------|
| **Pod** | A Kubernetes Pod running exactly one Aerospike server container (`aerospike-server`), plus optional sidecars (e.g., Prometheus exporter). |
| **Warm Restart** | Sending `SIGUSR1` to the `asd` process — restarts the server without losing in-memory data (CE 8.x+). Faster than a cold restart. |
| **Cold Restart** | Deleting and recreating the Pod. The Aerospike process starts fresh; in-memory data is lost. Required for pod spec changes. |
| **Rolling Restart** | The operator restarts pods one batch at a time (controlled by `spec.rollingUpdateBatchSize`) to maintain availability during config or image changes. |

## Configuration Concepts

| Term | Definition |
|------|-----------|
| **aerospikeConfig** | The `spec.aerospikeConfig` field — a free-form map that maps directly to `aerospike.conf`. The operator converts it to Aerospike's text config format. |
| **Dynamic Config Update** | Applying config changes at runtime via Aerospike's `set-config` command, without restarting the pod. Enabled by `spec.enableDynamicConfigUpdate: true`. |
| **Config Hash** | A SHA-256 hash of the effective Aerospike configuration, stored as an annotation (`acko.io/config-hash`) on each Pod. The operator uses it to detect which pods need restarting. |
| **PodSpec Hash** | A SHA-256 hash of the Pod specification, stored as `acko.io/podspec-hash`. Changes trigger a cold restart (pod deletion + recreation). |

## Common Abbreviations

| Abbreviation | Meaning |
|---|---|
| `asc` | Short name for `aerospikeclusters` (kubectl alias) |
| `asct` | Short name for `aerospikeclustertemplates` |
| `asd` | Aerospike server daemon — the Aerospike database process |
| `asinfo` | Aerospike info CLI tool for querying node state |
| `asadm` | Aerospike admin CLI tool for management operations |
| `PDB` | PodDisruptionBudget — K8s object limiting simultaneous pod disruptions |
| `PVC` | PersistentVolumeClaim — K8s storage request |
| `ACL` | Access Control List — Aerospike user/role permissions |
