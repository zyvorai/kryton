# Kryton architecture

Kryton is deliberately split at the **provider boundary**. Callers see one stable machine API; providers translate it into demo state, dockur compose stacks, or KubeVirt VirtualMachines.

```text
Consumers
  Veyron / Zeus / Atlas / Haven / CI / portals / third parties
                         |
                 REST + CloudEvents (+ OpenAPI / CORS)
                         |
                     Kryton API
                         |
             provider.Provider contract
                         |
        +---------+------+-----------+
        |         |                  |
     demo      dockur            KubeVirt
                  |                  |
           dockur/windows     Kubernetes REST
           (Docker/Podman+KVM)       |
                               QEMU / KVM VMs
                                     │
                          CSI disks (Rook / Longhorn)
                          optional Atlas discovery
```

---

## Providers

| Provider | Source of truth | Real Windows |
|----------|-----------------|--------------|
| **demo** | In-memory map | No — instant fake machines for eval |
| **dockur** | Compose state under `KRYTON_DOCKUR_DATA_DIR` | Yes — via [dockur/windows](https://github.com/dockur/windows) |
| **kubevirt** | Kubernetes API | Yes — operator golden images via CDI |

See [DOCKUR.md](DOCKUR.md) for the lab provider. See [DEPLOYMENT.md](DEPLOYMENT.md) for KubeVirt production.

---

## Stable identity

A Kryton machine receives a **UUID** independent from its provider name. The KubeVirt provider records the UUID and project in labels and preserves the original Kryton specification in a managed annotation. External clients therefore never need to address `namespace/name` directly.

The dockur provider maps UUIDs to compose project directories. The demo provider holds machines in a process-local map.

---

## Source of truth

- **KubeVirt** — Kubernetes is authoritative. Kryton is stateless with respect to machine inventory and can be restarted without losing machine identity.
- **demo** — intentionally in-memory; data is lost on restart.
- **dockur** — compose projects and disk images persist under `KRYTON_DOCKUR_DATA_DIR`.

---

## Projects

Kryton projects map to provider isolation domains:

- **KubeVirt** — one Kubernetes namespace per project (optionally prefixed by `KRYTON_NAMESPACE_PREFIX`).
- **dockur** — project is recorded on the machine; compose stacks are namespaced by machine ID.
- **demo** — logical grouping only.

RBAC roles (`viewer` · `operator` · `admin`) are always intersected with project membership.

---

## Events

Lifecycle events use the CloudEvents structured envelope. They are available through:

- `GET /api/v1/events` — history
- `GET /api/v1/events/stream` — authenticated SSE
- Optional webhook sink (`KRYTON_EVENT_WEBHOOK_URL`)

Dockur provisioning additionally emits `io.kryton.machine.install.started` when unattended setup begins.

---

## Diagnostics

`internal/doctor` runs provider-aware health checks exposed as `GET /api/v1/doctor` and `krytonctl doctor`. Checks include auth mode, projects, catalog, provider health, and (for dockur) runtime, compose, KVM, and data-dir writability. For kubevirt it also checks namespaces, DataSources, snapshot CRDs, and StorageClass ↔ VolumeSnapshotClass pairing.

`POST /api/v1/settings/test` and Settings → **Test connection** run live probes (Kubernetes, KubeVirt, storage, kubectl, install scripts).

## Storage and Atlas

KubeVirt VM disks need a CSI StorageClass with a matching VolumeSnapshotClass — see [STORAGE.md](STORAGE.md). Operators can install Rook/Longhorn from Settings or scripts, and set the Kryton default StorageClass via UI/API (`~/.kryton/storage.json`).

Optional **Atlas** integration ([ATLAS.md](ATLAS.md)) points Kryton at the Zyvor storage control plane (`product: kryton`) for discovery and ownership conventions.

## Public API surface

Other products discover Kryton via `GET /api/v1` and `GET /openapi.yaml`. Cross-origin browser clients need `KRYTON_CORS_ORIGINS`. See [API.md](API.md).

---

## Security boundaries

Kryton does not expose raw QEMU, RDP, WinRM, or arbitrary PowerShell execution through the API.

- **API keys** for service-to-service use (SHA-256 digests stored, raw tokens never persisted).
- **Proxy auth** for browser SSO via trusted reverse proxy headers.
- Production KubeVirt mode refuses to start with authentication disabled unless the operator explicitly opts into insecure operation (`KRYTON_ALLOW_INSECURE=true`).
- The dockur provider is intended for labs; use API-key auth on shared hosts.

Console and RDP ports for dockur are published on the host — restrict with firewall policy.
