---
sidebar_position: 4
title: ackoctl CLI
---

import Tabs from '@theme/Tabs';
import TabItem from '@theme/TabItem';

# ackoctl — Command-line interface

[ackoctl](https://github.com/aerospike-ce-ecosystem/ackoctl) provides terminal and CI access to the same operations as the [Cluster Manager UI](./cluster-manager-ui.md). Both call the `aerospike-cluster-manager` REST API at `/api/v1/*`.

The Go CLI follows the `kubectl` and `gh` command pattern: `ackoctl <noun> <verb>`. It does **not** connect directly to Kubernetes or Aerospike; it connects to the cluster-manager API deployment.

---

## Install

<Tabs groupId="ackoctl-install" queryString="install">
<TabItem value="homebrew" label="Homebrew (macOS, Linux)" default>

```bash
brew install aerospike-ce-ecosystem/tap/ackoctl
```

</TabItem>
<TabItem value="posix" label="Any POSIX shell">

```bash
curl -fsSL https://raw.githubusercontent.com/aerospike-ce-ecosystem/ackoctl/main/install.sh | sh
```

</TabItem>
</Tabs>

Verify:

```bash
ackoctl version
```

See the [ackoctl install guide](https://github.com/aerospike-ce-ecosystem/ackoctl/blob/main/docs/install.md) for pinning a version, custom install directories, and source builds.

---

## Configure access to cluster-manager

`ackoctl` reads `~/.ackoctl/config.yaml`, the same kubeconfig-style multi-context layout that `kubectl` uses.

If your cluster-manager is running inside the same Kubernetes cluster (the default for ACKO chart 0.4.0+), expose it via `kubectl port-forward` first:

```bash
kubectl -n aerospike-operator port-forward \
  svc/acko-aerospike-ce-kubernetes-operator-ui 8000:8000
```

Then point `ackoctl` at it:

```bash
ackoctl config set-context kind-local \
  --server=http://localhost:8000/api \
  --workspace-id=default
ackoctl config use-context kind-local
ackoctl config view
```

For remote cluster-managers, drop the port-forward and use the public URL:

```bash
ackoctl config set-context prod \
  --server=https://acm.example.com/api \
  --token="$(your-oidc-flow)" \
  --workspace-id=production
ackoctl config use-context prod
```

`ackoctl` has no `login` command — OIDC tokens are obtained out-of-band (Keycloak CLI, browser device flow, etc.) and passed via `--token` or the `ACKOCTL_TOKEN` environment variable.

Override precedence: **CLI flag > environment variable > config file**.

---

## Command map

```
ackoctl
├── version                                       binary version, commit, build date
├── config       view | set-context | use-context | current-context | delete-context
├── connection   list | get | create | update | delete | health
├── cluster      info | configure-namespace
├── k8s cluster  list | get | reconcile
├── record       list | get | put | delete | query
├── set          list
├── query        exec
└── index        list | create | delete
```

Every list/get command supports `-o table|json|yaml` so the output is pipeable into `jq`, `yq`, or scripts.

---

## Typical workflows

### Inspect ACKO-managed clusters

```bash
# List every AerospikeCluster CR ACKO sees in the workspace
ackoctl k8s cluster list

# Drill into one
ackoctl k8s cluster get my-cluster -o yaml

# Force a reconcile (e.g. after editing the CR by hand)
ackoctl k8s cluster reconcile my-cluster
```

### Register an Aerospike connection and browse data

```bash
ackoctl connection create \
  --name local-aero \
  --host aerospike-node-1 --host aerospike-node-2 \
  --port 3000 \
  --namespace test

ackoctl connection health local-aero

ackoctl set list --connection local-aero --namespace test
ackoctl record list --connection local-aero --namespace test --set users --limit 20
```

### Run an ad-hoc query

```bash
ackoctl query exec \
  --connection local-aero \
  --namespace test \
  --set users \
  --where 'age > 30' \
  --select name,email \
  -o json
```

### Manage secondary indexes

```bash
ackoctl index list --connection local-aero --namespace test
ackoctl index create --connection local-aero --namespace test --set users \
  --name idx_age --bin age --type numeric
ackoctl index delete --connection local-aero --namespace test --name idx_age
```

---

## Use inside CI / automation

Every operation that touches mutating state respects `--workspace`, which is the cluster-manager ACL boundary. Pair that with a short-lived OIDC token and you can drive ACKO reconciles from any pipeline:

```bash
export ACKOCTL_SERVER=https://acm.example.com/api
export ACKOCTL_TOKEN="${CI_OIDC_TOKEN}"
export ACKOCTL_WORKSPACE=production

ackoctl k8s cluster reconcile my-cluster
ackoctl k8s cluster get my-cluster -o json | jq -r '.status.phase'
```

Errors from cluster-manager surface as structured `{"detail": "..."}` messages and exit code 1, so `set -e` works as expected.

---

## More

- Source repository — [aerospike-ce-ecosystem/ackoctl](https://github.com/aerospike-ce-ecosystem/ackoctl)
- Full command reference — [`docs/usage.md`](https://github.com/aerospike-ce-ecosystem/ackoctl/blob/main/docs/usage.md)
- Install options — [`docs/install.md`](https://github.com/aerospike-ce-ecosystem/ackoctl/blob/main/docs/install.md)
- Release notes — [`CHANGELOG.md`](https://github.com/aerospike-ce-ecosystem/ackoctl/blob/main/CHANGELOG.md)
- Cluster Manager UI (web counterpart) — [Cluster Manager UI guide](./cluster-manager-ui.md)
