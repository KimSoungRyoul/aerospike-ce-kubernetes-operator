# scripts/keycloak

Local / e2e Keycloak realm bootstrap. **Not for production.**

## Files

| File | Role |
|------|------|
| `values-keycloak-local.yaml` | helm values for `bitnami/keycloak` 25.2.0. Overrides every image to the `bitnamilegacy/*` mirror (since bitnami removed the free tags on 2025-08-28) and points `keycloakConfigCli` at the `acko-realm` ConfigMap. Pair with `--version 25.2.0` on the helm CLI. |
| `acko-realm.json` | Realm definition imported by `keycloakConfigCli`. |
| `README.md` | This file. |

## Why a separate values file?

`Makefile install-keycloak` and any operator who wants to tweak the
local IdP (e.g. enabling smtp, raising postgres pvc size) share a
single source of truth. The Makefile only owns the chart version pin
and the namespace; everything else lives in
`values-keycloak-local.yaml`. To override locally without editing the
file, drop a sibling `values-keycloak-local.override.yaml` and pass
both via `helm upgrade -f values-keycloak-local.yaml -f
values-keycloak-local.override.yaml`.

## `acko-realm.json` — DEV-ONLY

This file is imported by `keycloakConfigCli` (sidecar of bitnami/keycloak)
during `make run-local` and CI e2e to provision the `acko` realm with the
clients, roles, audience mapper, and test users that the e2e scenarios
rely on.

> [!WARNING]
> **DO NOT apply this realm to a production Keycloak.**
>
> - Plaintext credentials: `admin/admin`, `dev-user/dev`, `prod-user/prod`
> - Wildcard `redirectUris: ["*"]` and `webOrigins: ["*"]`
> - `directAccessGrantsEnabled: true` on the public SPA client (so e2e
>   tests can fetch tokens via Resource Owner Password grant — never
>   enable this on a public-internet-facing IdP).
> - The `acko-other` client exists solely so e2e can issue tokens with
>   the wrong audience and assert the API rejects them.
>
> For production, follow `aerospike-ce-kubernetes-operator/docs/multi-cluster-keycloak.md`
> which documents the kcadm and Terraform Keycloak provider paths.

The file lives in two places that must stay in sync:

| Path | Consumer |
|------|----------|
| `scripts/keycloak/acko-realm.json` | `make install-keycloak` → ConfigMap `acko-realm` |
| `test/utils/testdata/acko-realm.json` | Go `//go:embed` for e2e helpers |

`scripts/keycloak/` is the source of truth; `test/utils/testdata/` is a
copy. See `MULTI_CLUSTER_OIDC_FOLLOW_UPS.md` for the open question about
collapsing the duplication via `//go:embed` with a relative path.

## Why no top-level `_comment` field in the JSON?

Keycloak's `keycloak-config-cli` uses Jackson with strict schema
validation, which rejects unknown fields (`UnrecognizedPropertyException`).
The realm JSON must contain only fields from the official Keycloak
RealmRepresentation; comments belong here in this README.
