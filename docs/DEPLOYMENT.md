# Production deployment

For a lab host over SSH (demo or dockur provider + systemd), see **[DEPLOY-REMOTE.md](DEPLOY-REMOTE.md)** and **[DOCKUR.md](DOCKUR.md)**.

For automated Windows 11 on KubeVirt, see **[KUBEVIRT.md](KUBEVIRT.md)** (`./scripts/setup-kubevirt.sh`).

This guide covers the **KubeVirt** production path with Helm.

---

## Prerequisites

- Kubernetes with KubeVirt installed.
- CDI available for image-backed DataVolume templates.
- `DataSource` objects for each configured Kryton image ID in the configured image namespace.
- One Kubernetes namespace per Kryton project, or namespaces using the configured prefix.
- RBAC allowing the Kryton service account to manage KubeVirt VMs and snapshots in those namespaces.
- A **CSI StorageClass with VolumeSnapshotClass** for VM disks (Rook Ceph RBD recommended, Longhorn for lab). See **[STORAGE.md](STORAGE.md)**.
- (Optional) Atlas gateway for suite-wide storage ownership — **[ATLAS.md](ATLAS.md)**.
- OpenAPI served at `/openapi.yaml`; set `KRYTON_CORS_ORIGINS` when browsers on other Zyvor apps call Kryton.

Validate your estate before go-live:

```bash
krytonctl doctor   # after pointing KRYTON_PROVIDER=kubevirt at the cluster
```

---

## Settings & storage

From the Kryton UI (**Settings**):

- **Test connection** — Kubernetes / KubeVirt / storage probes (`POST /api/v1/settings/test`)
- **Operator settings** — default project, image namespace, storage class, event webhook
- **Atlas** — optional Zyvor storage control plane ([ATLAS.md](ATLAS.md))
- **Cluster storage** — install Rook/Longhorn and pick the default StorageClass ([STORAGE.md](STORAGE.md))

Helm values: `corsOrigins`, `storageClass`, and `-f values-rook-ceph.yaml` / `values-longhorn.yaml`.

---

## Image contract

Kryton does not distribute Microsoft installation media or licenses. An image catalog entry is a logical reference. The KubeVirt provider expects a CDI `DataSource` with the same ID.

Example:

```yaml
apiVersion: cdi.kubevirt.io/v1beta1
kind: DataSource
metadata:
  name: windows-server-2025
  namespace: kryton-images
spec:
  source:
    pvc:
      name: windows-server-2025-golden
      namespace: kryton-images
```

---

## Helm install

```bash
export KRYTON_PROVIDER=kubevirt
export KRYTON_PROJECTS=finance,engineering
export KRYTON_DEFAULT_PROJECT=finance
export KRYTON_IMAGE_NAMESPACE=kryton-images
export KRYTON_AUTH_MODE=apikey
export KRYTON_API_KEYS_FILE=/etc/kryton/keys.json

TOKEN=$(krytonctl generate-token)
krytonctl hash-token "$TOKEN"   # store only the hash in keys.json

kubectl -n kryton create secret generic kryton-auth --from-file=keys.json
helm upgrade --install kryton ./deploy/helm/kryton -n kryton --create-namespace
```

Image: `ghcr.io/zyvorai/kryton` (see `deploy/helm/kryton/values.yaml`).

Storage profiles:

```bash
# Rook Ceph RBD (after scripts/enable-rook-ceph.sh)
helm upgrade --install kryton ./deploy/helm/kryton -n kryton -f deploy/helm/kryton/values-rook-ceph.yaml

# Longhorn lab CSI
helm upgrade --install kryton ./deploy/helm/kryton -n kryton -f deploy/helm/kryton/values-longhorn.yaml
```

---

## Observability & availability

- `GET /metrics` exposes Prometheus-text counters. Set `serviceMonitor.enabled: true` in the Helm chart to have a Prometheus Operator scrape it automatically.
- `podDisruptionBudget.enabled: true` protects against voluntary disruption — but only enable it once running more than one `krytond` replica is actually safe (see below).
- **Run exactly one `krytond` replica.** The TTL reconciler has no leader election and the event bus is in-process, so more than one replica is unsafe today. Full details: [ARCHITECTURE.md § Single-replica today](ARCHITECTURE.md#single-replica-today), [deploy/helm/kryton/README.md § Single-replica today](../deploy/helm/kryton/README.md#single-replica-today).
- Set `KRYTON_RATE_LIMIT_RPS`/`KRYTON_RATE_LIMIT_BURST` (Helm: `rateLimit.rps`/`.burst`) to protect the API from a noisy caller; see [API.md § Rate limiting](API.md#rate-limiting).

---

## Authentication

For machine-to-machine access, use `KRYTON_AUTH_MODE=apikey` and mount a JSON key file from a Kubernetes Secret. Store SHA-256 token digests rather than raw tokens.

For browser SSO, place Kryton behind an authenticated reverse proxy and use `KRYTON_AUTH_MODE=proxy`. The proxy must strip inbound `X-Kryton-*` headers, set trusted identity headers itself, and send the shared `X-Kryton-Proxy-Secret` value loaded by Kryton from `KRYTON_PROXY_SECRET_FILE`. Do not expose the Kryton service directly when proxy auth is enabled.

---

## TLS

Kryton can terminate TLS directly with `KRYTON_TLS_CERT_FILE` and `KRYTON_TLS_KEY_FILE`. If `KRYTON_CLIENT_CA_FILE` is configured, the server additionally requires and verifies client certificates.

Alternatively, terminate TLS at an ingress controller and forward plain HTTP to Kryton inside the cluster. Enable the chart Ingress:

```yaml
ingress:
  enabled: true
  className: nginx
  hosts:
    - host: kryton.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: kryton-tls
      hosts: [kryton.example.com]
```

See **[GA.md](GA.md)** for the production go-live checklist.

---

## Network attachments

When `network.networkId` is empty, the KubeVirt provider creates a pod network with masquerade binding. When it is set, the value is passed as a Multus network attachment name and Kryton uses bridge binding.

---

## TTL

`ttlMinutes` is persisted as a provider annotation. The TTL reconciler periodically lists machines and deletes expired workloads. This remains effective across Kryton restarts.

---

## Provider comparison

| | demo | dockur | kubevirt |
|---|------|--------|----------|
| **Target** | Local eval | Lab Linux host | Production K8s |
| **Persistence** | In-memory | Compose on disk | Kubernetes etcd |
| **Auth default** | disabled | disabled (use apikey) | apikey required |
| **Real Windows** | No | Yes | Yes (golden images) |
