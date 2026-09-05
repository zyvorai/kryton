# Kryton Helm chart

Deploys `krytond` to Kubernetes with the `kubevirt` provider — the production path described in [docs/DEPLOYMENT.md](../../../docs/DEPLOYMENT.md) and [docs/ARCHITECTURE.md](../../../docs/ARCHITECTURE.md).

```bash
helm upgrade --install kryton ./deploy/helm/kryton -n kryton --create-namespace \
  -f deploy/helm/kryton/values-customer.yaml
```

## Values files

| File | Use case |
|------|----------|
| [`values.yaml`](values.yaml) | Chart defaults — starting point, no overlay needed for reference |
| [`values-lab.yaml`](values-lab.yaml) | Shared lab/demo cluster: auth **disabled**, NodePort `30088`, local image tag |
| [`values-customer.yaml`](values-customer.yaml) | Production: `apikey` auth, ingress + TLS, `rook-ceph-block` storage |
| [`values-rook-ceph.yaml`](values-rook-ceph.yaml) | Overlay-only: point `storageClass` at Rook Ceph RBD (snapshots + clones) |
| [`values-longhorn.yaml`](values-longhorn.yaml) | Overlay-only: point `storageClass` at Longhorn CSI (lab snapshots) |

The two storage overlays are meant to be layered on top of `values.yaml` or `values-customer.yaml` with `-f`, not used alone — see [docs/STORAGE.md](../../../docs/STORAGE.md) for the Rook Ceph vs. Longhorn tradeoffs and `scripts/enable-rook-ceph.sh` / `scripts/enable-kubevirt-snapshots.sh` for installing the storage layer itself.

## Key values

| Key | Default | Notes |
|-----|---------|-------|
| `provider` | `kubevirt` | The chart only wires up the KubeVirt provider; `demo`/`dockur` are for `make demo` / bare-metal labs, not Helm |
| `projects` / `defaultProject` | `[default]` / `default` | Maps to `KRYTON_PROJECTS` / `KRYTON_DEFAULT_PROJECT` — one KubeVirt namespace per project |
| `namespacePrefix` | `""` | Prefixes the per-project namespace Kryton creates/manages |
| `imageNamespace` | `kryton-images` | Namespace holding operator-managed golden-image `DataSource` objects |
| `storageClass` | `""` (cluster default) | PVC StorageClass for new VM disks; set via overlay or `KRYTON_STORAGE_CLASS` |
| `auth.mode` | `apikey` | `disabled` · `apikey` · `proxy` — chart **requires** `auth.existingSecret` unless `disabled` |
| `auth.existingSecret` | `kryton-auth` | Secret holding `keys.json` (apikey) or a proxy shared secret; create it before `helm install` (see below) |
| `auth.trustProxy` / `auth.allowInsecure` | `false` | Proxy-auth mode and lab TLS-skip escape hatches — leave both `false` in production |
| `eventWebhookURL` | `""` | Optional CloudEvents webhook sink |
| `corsOrigins` | `[]` | Browser origins allowed to call the API cross-origin (portals, dashboards) |
| `reconcileInterval` | `30s` | TTL-expiry sweep interval |
| `rateLimit.rps` / `.burst` | `0` / `0` | Per-caller `/api/*` token bucket; `0` disables rate limiting |
| `serviceMonitor.enabled` | `false` | Renders a Prometheus Operator `ServiceMonitor` scraping `/metrics`; requires the Prometheus Operator CRDs in-cluster |
| `podDisruptionBudget.enabled` | `false` | Renders a `PodDisruptionBudget`; only meaningful once `replicaCount > 1` is safe — see the note below |
| `ingress.*` | disabled | Standard `networking.k8s.io/v1` Ingress; only used when `ingress.enabled: true` |
| `service.type` / `service.nodePort` | `ClusterIP` / `0` | Set `NodePort` + a port for lab access without an Ingress controller |
| `rbac.clusterWide` | `true` | `true` → ClusterRole (multi-namespace/multi-project); `false` → namespaced Role scoped to the release namespace |
| `images` | 3 sample entries | Static catalog entries (`KRYTON_IMAGES_FILE`) describing selectable images in the UI/CLI — the backing `DataSource` still has to exist in `imageNamespace`, see [docs/GOLDEN-IMAGES.md](../../../docs/GOLDEN-IMAGES.md) |
| `resources` / `podSecurityContext` / `containerSecurityContext` | hardened defaults | Non-root (explicit numeric `runAsUser: 65532`/`runAsGroup: 65532` — required for kubelets that can't verify `runAsNonRoot` against the image's named `nonroot` user), read-only rootfs, all capabilities dropped |
| `networkPolicy.enabled` | `false` | When on, restricts ingress to port 8080 and allows all egress (KubeVirt/API-server access) |

## Ephemeral local state

`readOnlyRootFilesystem: true` leaves krytond's default `$HOME/.kryton/{storage,settings}.json` unwritable, so the chart points `KRYTON_STORAGE_CONFIG_FILE`/`KRYTON_SETTINGS_CONFIG_FILE` at `/var/lib/kryton`, backed by an `emptyDir`. That means the operator-set default StorageClass and Settings-UI values (Atlas config, event webhook URL, etc.) do **not** survive a pod restart or reschedule — they reset to the Helm-configured defaults (`storageClass`, `eventWebhookURL` values) each time. This is consistent with the single-replica-only posture below; a durable fix would need a small PVC or a ConfigMap/Secret written back through the Kubernetes API instead of a local file.

## Single-replica today

Do not set `replicaCount > 1`. `internal/reconciler/ttl.go` has no leader election, so every replica would independently expire the same machines, and `internal/events/events.go`'s event history/SSE fan-out is purely in-process, so different replicas would show different event streams to different clients. `podDisruptionBudget` is included but left disabled by default for the same reason — flip both on only after that gap is closed. See [docs/ARCHITECTURE.md](../../../docs/ARCHITECTURE.md).

## Auth secret

`auth.existingSecret` (default `kryton-auth`) must exist in the release namespace before install whenever `auth.mode` is not `disabled` — the chart does not create it. For `apikey` mode:

```bash
TOKEN=$(krytonctl generate-token)
HASH=$(krytonctl hash-token "$TOKEN")
# Keep TOKEN in your secret manager — only the hash goes in the cluster.
kubectl -n kryton create secret generic kryton-auth \
  --from-literal=keys.json="{\"keys\":[{\"name\":\"ops\",\"sha256\":\"$HASH\",\"role\":\"admin\",\"projects\":[\"*\"]}]}"
echo "Export for CLI/CI: export KRYTON_TOKEN=$TOKEN"
```

Lab hosts (non-Helm): `./scripts/ensure-api-keys.sh` then `cat ~/.kryton/lab.token`. Full guide: [docs/AUTH.md](../../../docs/AUTH.md).

For `proxy` mode, the same secret instead holds a proxy shared-secret file — see [docs/API.md](../../../docs/API.md).

## RBAC

The chart grants `krytond` `get/list/watch` on KubeVirt `VirtualMachines`, `VirtualMachineInstances`, snapshot/restore CRDs, `StorageClasses`, `VolumeSnapshotClasses`, and CDI `DataSources`, plus `create` on `namespaces` and lifecycle verbs on VMs/snapshots. `rbac.clusterWide: false` swaps the `ClusterRole`/`ClusterRoleBinding` for a namespaced `Role`/`RoleBinding` — use that only when every project's namespace equals the release namespace.

## What the chart does not manage

- KubeVirt/CDI installation itself — see [docs/KUBEVIRT.md](../../../docs/KUBEVIRT.md) and `scripts/setup-kubevirt-production.sh`.
- Golden-image creation — see [docs/GOLDEN-IMAGES.md](../../../docs/GOLDEN-IMAGES.md).
- The storage layer (Rook Ceph / Longhorn install) — see `deploy/rook-ceph/` and [docs/STORAGE.md](../../../docs/STORAGE.md).
