# Multi-Cluster Topology with Keycloak OIDC

This guide describes how to install ACKO in a **multi-cluster topology**: one **common cluster** that hosts only the web UI (cluster-manager SPA) and one or more **operator clusters** (typically `dev` and `prod`) that each run the API + operator and manage their own local Aerospike clusters. Authentication is delegated to an **external Keycloak realm**, and FastAPI verifies access tokens natively via JWKS.

This is the production topology behind ADR-0040.

---

## Topology

```
                 +-------------------------------+
                 |   Browser (developer / SRE)   |
                 +---------------+---------------+
                                 |
              +------------------+------------------+
              |                  |                  |
              v                  v                  v
    +------------------+  +-----------------+  +------------------+
    |  common cluster  |  |   dev cluster   |  |  prod cluster    |
    |  (web only)      |  |  (api+operator) |  |  (api+operator)  |
    |                  |  |                 |  |                  |
    |  ingress: app.*  |  | ingress: api.   |  | ingress: api.    |
    |  /cluster-       |  |  dev.*          |  |  prod.*          |
    |  registry.json   |  |                 |  |                  |
    +--------+---------+  +--------+--------+  +---------+--------+
             |                     |                     |
             |                     v                     v
             |             +---------------+    +----------------+
             |             | Aerospike CE  |    | Aerospike CE   |
             |             | dev cluster   |    | prod cluster   |
             |             +---------------+    +----------------+
             |
             v  (authentication only — not data)
       +-----------------+
       |   Keycloak      |    realm: acko
       |   (external)    |    SPA client: acko-spa (PKCE)
       +-----------------+    audience: acko-api
```

Key properties:

- The **browser is the fan-out point**. After loading the SPA from the common cluster, it calls each operator cluster's API ingress directly. There is no proxy hop in the common cluster.
- The **cluster registry** (which operator clusters exist, what their host names are, what label/role gates them) is shipped from helm values via a ConfigMap mounted into the web pod at `/cluster-registry.json`. There is no runtime registration path.
- **Authentication** is performed by FastAPI itself: each operator cluster's API verifies the JWT (`audience: acko-api`) against the Keycloak realm's JWKS. There is no shared session storage across clusters.
- **Authorization** uses Keycloak realm roles. Cluster-scoped roles follow the `acko:<env>` naming convention (`acko:dev`, `acko:prod`).

---

## Prerequisites

| Requirement | Notes |
|-------------|-------|
| 1 common Kubernetes cluster | Hosts the web UI |
| 1+ operator Kubernetes clusters | Each hosts api+operator and one Aerospike CE cluster |
| Keycloak (production-grade) | External, reachable from the browser AND from each operator cluster's pods |
| cert-manager on every cluster | Wildcard or per-host TLS via LetsEncrypt is recommended |
| DNS for each ingress host | `app.example.com`, `api.dev.example.com`, `api.prod.example.com`, etc. |
| Helm 3.8+ | OCI registry support required |

> **Note**: Local/CI bootstrap of Keycloak via `bitnami/keycloak` is supported for development and e2e but is **not** recommended for production. In production, run Keycloak as a separately managed service.

---

## Step 1 — Set up Keycloak (realm, client, audience mapper)

ACKO uses a **single realm**, a **single SPA client**, and a **single API audience**. Cluster-level isolation is expressed as realm roles, not as separate audiences.

### Required objects

| Object | Value |
|--------|-------|
| Realm | `acko` |
| SPA client | `acko-spa` — public, standard flow + PKCE, no client secret |
| API audience | `acko-api` — added via a hardcoded audience mapper on `acko-spa` |
| Roles | `acko:dev`, `acko:prod` (optional, only if you want per-environment gating) |

### Option A — `kcadm.sh` (Keycloak CLI)

```sh
# Authenticate (replace KEYCLOAK_URL and admin password)
kcadm.sh config credentials \
  --server "$KEYCLOAK_URL" \
  --realm master \
  --user admin \
  --password "$KEYCLOAK_ADMIN_PASSWORD"

# Create the realm
kcadm.sh create realms -s realm=acko -s enabled=true

# Create the SPA public client with PKCE
kcadm.sh create clients -r acko \
  -s clientId=acko-spa \
  -s publicClient=true \
  -s standardFlowEnabled=true \
  -s 'attributes."pkce.code.challenge.method"=S256' \
  -s 'redirectUris=["https://app.example.com/*"]' \
  -s 'webOrigins=["+"]'

# Add a hardcoded audience mapper so tokens carry aud=acko-api
SPA_ID=$(kcadm.sh get clients -r acko -q clientId=acko-spa --fields id --format csv --noquotes | tail -n1)
kcadm.sh create clients/$SPA_ID/protocol-mappers/models -r acko \
  -s name=acko-api-audience \
  -s protocol=openid-connect \
  -s protocolMapper=oidc-audience-mapper \
  -s 'config."included.custom.audience"=acko-api' \
  -s 'config."access.token.claim"=true'

# Create cluster-scoped realm roles (optional)
kcadm.sh create roles -r acko -s name=acko:dev
kcadm.sh create roles -r acko -s name=acko:prod
```

### Option B — Terraform Keycloak provider

```hcl
terraform {
  required_providers {
    keycloak = {
      source  = "keycloak/keycloak"
      version = "~> 4.4"
    }
  }
}

provider "keycloak" {
  url       = var.keycloak_url
  client_id = "admin-cli"
  username  = var.keycloak_admin
  password  = var.keycloak_password
  realm     = "master"
}

resource "keycloak_realm" "acko" {
  realm   = "acko"
  enabled = true
}

resource "keycloak_openid_client" "spa" {
  realm_id                     = keycloak_realm.acko.id
  client_id                    = "acko-spa"
  access_type                  = "PUBLIC"
  standard_flow_enabled        = true
  pkce_code_challenge_method   = "S256"
  valid_redirect_uris          = ["https://app.example.com/*"]
  web_origins                  = ["+"]
}

resource "keycloak_openid_audience_protocol_mapper" "spa_audience" {
  realm_id                 = keycloak_realm.acko.id
  client_id                = keycloak_openid_client.spa.id
  name                     = "acko-api-audience"
  included_custom_audience = "acko-api"
  add_to_access_token      = true
}

resource "keycloak_role" "dev" {
  realm_id = keycloak_realm.acko.id
  name     = "acko:dev"
}

resource "keycloak_role" "prod" {
  realm_id = keycloak_realm.acko.id
  name     = "acko:prod"
}
```

### Verify the realm

```sh
curl -s "$KEYCLOAK_URL/realms/acko/.well-known/openid-configuration" | jq '.issuer, .jwks_uri'
```

You should see the realm issuer and a JWKS URI. Both must be reachable from the browser **and** from every operator cluster's API pod.

---

## Step 2 — Install ACKO on each cluster

> File names and values keys below match the contracts agreed in ADR-0040. Refer to the chart's `values.yaml` for the complete schema.

### Common cluster (web only)

`values-common.yaml`:

```yaml
operator:
  enabled: false        # no operator on the common cluster
ui:
  api:
    enabled: false      # no API on the common cluster
  web:
    enabled: true
    auth:
      oidc:
        issuerUrl: https://keycloak.example.com/realms/acko
        clientId: acko-spa
multiCluster:
  enabled: true
  clusters:
    - name: dev
      label: Development
      apiBaseUrl: https://api.dev.example.com
      requiredRole: acko:dev   # optional — omit for "any authenticated user"
    - name: prod
      label: Production
      apiBaseUrl: https://api.prod.example.com
      requiredRole: acko:prod
```

```sh
helm install acko-web oci://ghcr.io/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator \
  -n acko-web --create-namespace \
  -f values-common.yaml
```

### Operator cluster (dev / prod)

`values-operator.yaml` (per environment — change `name`/issuer/audience as needed):

```yaml
operator:
  enabled: true
ui:
  api:
    enabled: true
    auth:
      oidc:
        issuerUrl: https://keycloak.example.com/realms/acko
        audience: acko-api
        # JWKS endpoint is auto-derived from the realm's
        # /.well-known/openid-configuration document.
  web:
    enabled: false       # web lives on the common cluster only
multiCluster:
  enabled: false         # operator clusters do not host the registry
```

```sh
helm install acko oci://ghcr.io/aerospike-ce-ecosystem/aerospike-ce-kubernetes-operator \
  -n acko --create-namespace \
  -f values-operator.yaml
```

Repeat for each environment (`dev`, `prod`, …).

---

## Step 3 — DNS and TLS

Each cluster needs its own ingress host, and each host needs its own TLS certificate. The recommended pattern is **cert-manager + LetsEncrypt** with a per-cluster `ClusterIssuer`.

| Cluster | Ingress host | Certificate |
|---------|-------------|-------------|
| common  | `app.example.com` | LetsEncrypt via cert-manager |
| dev     | `api.dev.example.com` | LetsEncrypt via cert-manager |
| prod    | `api.prod.example.com` | LetsEncrypt via cert-manager |

Cross-origin requirements: every API ingress must allow `Origin: https://app.example.com` (the common cluster) in CORS, otherwise the browser fan-out to dev/prod will be blocked. The chart sets this from `multiCluster.clusters[].apiBaseUrl` (and from the web ingress host).

---

## Troubleshooting

### `401 Unauthorized` from the API

1. The browser must be sending an `Authorization: Bearer <token>` header. Open DevTools → Network and confirm the header is present and the token is not expired.
2. Decode the token (jwt.io). Check:
   - `iss` matches the API's configured `issuerUrl`
   - `aud` includes `acko-api` (this is what the audience mapper does — if it is missing, the mapper is not attached to `acko-spa`)
3. If the role gate is enabled, check `realm_access.roles[]` includes the cluster-scoped role (`acko:dev` or `acko:prod`).

### `JWKS fetch failed` in API logs

- The API pod cannot reach `https://keycloak.example.com/realms/acko/protocol/openid-connect/certs`. Check:
  - Egress NetworkPolicy is not blocking outbound HTTPS to the Keycloak host
  - DNS resolves inside the cluster (`kubectl exec ... -- nslookup keycloak.example.com`)
  - The Keycloak certificate chain is trusted by the pod (custom CA? mount it into `/etc/ssl/certs`)

### CORS preflight fails on browser fan-out

- The dev/prod API ingress is rejecting `OPTIONS` from `app.example.com`. Verify the API chart values include the common cluster host in `ui.api.cors.allowedOrigins`. If the chart fills this from `multiCluster.clusters[].apiBaseUrl`, double-check the URL has no trailing slash mismatch.

### Cluster registry shows empty list

- The web pod cannot find `/cluster-registry.json`. Verify:
  - `multiCluster.enabled: true` AND `multiCluster.clusters[]` is non-empty in the common cluster's values
  - The `cluster-registry` ConfigMap exists in the web namespace (`kubectl get cm cluster-registry`)
  - The web pod has the ConfigMap mounted as a volume (`kubectl describe pod ...`)

### Token works on dev but not prod (or vice versa)

- Audience is shared (`acko-api`) but cluster-scoped roles are not. Ensure the user has both `acko:dev` and `acko:prod` if they need access to both, or remove the `requiredRole` gate to fall back to "any authenticated user".

---

## Open Items (Future Work)

This guide covers stage 1 only. The following items are tracked in ADR-0040 as future work:

- **Defense in depth via ingress oauth2-proxy / Keycloak gatekeeper** — currently FastAPI does the JWT verification by itself; an ingress-level proxy can be added in front later without breaking this flow.
- **Realm/client provisioning automation** — Terraform Keycloak provider HCL above is the recommended starting point.
- **mTLS between proxy and API** — service mesh or cert-manager based.
- **Cross-cluster metrics/logs federation** — Prometheus federation, Loki multi-tenant.
- **Multi-Kind e2e (stage 2)** — current e2e simulates the topology in a single Kind cluster via namespaces.
- **Cross-cluster PostgreSQL connection-profile aggregation** — each operator cluster currently keeps its own DB silo.
- **Per-cluster audiences (`acko-api-dev`, `acko-api-prod`)** — to harden against token replay across environments.

---

## References

- [ADR-0040](https://aerospike-ce-ecosystem.github.io/project-hub/docs/architecture/adr/2026-05-05-multi-cluster-topology-and-keycloak-oidc) — design rationale
- [Keycloak Server Administration Guide](https://www.keycloak.org/docs/latest/server_admin/)
- [Keycloak — Securing Applications Overview](https://www.keycloak.org/securing-apps/overview)
- [bitnami/keycloak Helm chart](https://github.com/bitnami/charts/tree/main/bitnami/keycloak)
- [cert-manager](https://cert-manager.io/docs/)
- [Terraform Keycloak Provider](https://registry.terraform.io/providers/keycloak/keycloak/latest/docs)
