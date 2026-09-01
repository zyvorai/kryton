# Kryton GA path (KubeVirt)

Production Kryton is the **kubevirt** provider behind Helm, hashed API keys, and TLS (ingress or process certificates). Dockur on `:7088` remains a lab installer, not GA.

## Must-have (implemented)

| Area | What GA requires |
|------|------------------|
| Snapshots | Create, list, restore, delete via API, UI, and `krytonctl` |
| Disk storage | CSI StorageClass with VolumeSnapshotClass (`rook-ceph-block` or `longhorn`) — see [STORAGE.md](STORAGE.md); Settings install + default |
| Images | Catalog IDs match CDI `DataSource` objects; golden → `POST /api/v1/golden/{id}/bootstrap` |
| Doctor | Auth, catalog, namespaces, DataSources, snapshot CRDs, StorageClass ↔ snapshot class |
| Settings | Runtime config, connection test, Atlas integration ([ATLAS.md](ATLAS.md)) |
| API | Public discovery `GET /api/v1`, OpenAPI `/openapi.yaml`, CORS for suite products |
| Helm | ClusterRole includes `virtualmachinerestores`; optional Ingress; `auth.mode: apikey` |
| Images | CI publishes `ghcr.io/zyvorai/kryton` on `main` |
| Events | Dashboard subscribes to `GET /api/v1/events/stream` with the same bearer token as REST |
| Auth | Never `disabled` on kubevirt; store SHA-256 digests in `keys.json` |

## Operator checklist

1. KubeVirt + CDI + snapshot feature gate (`./scripts/enable-kubevirt-snapshots.sh --skip-storage` or with a CSI backend).
2. Snapshot-capable disks: **Rook Ceph** (`./scripts/enable-rook-ceph.sh`) or **Longhorn** (`./scripts/enable-kubevirt-snapshots.sh --storage longhorn`). See [STORAGE.md](STORAGE.md).
3. Sysprepped qcow2 published as DataSources in `kryton-images` (same `KRYTON_STORAGE_CLASS` as VMs).
4. Match **virtctl** to the cluster KubeVirt version before `virtctl image-upload`.
5. `KRYTON_STORAGE_CLASS=rook-ceph-block` (or `longhorn`). Do not snapshot VMs on `rancher.io/local-path`.
6. `helm upgrade --install` with `provider: kubevirt`, `auth.mode: apikey`, Ingress TLS, and `-f values-rook-ceph.yaml` (or `values-longhorn.yaml`).
7. `krytonctl doctor` reports no `fail` findings.
8. Create a machine, snapshot, restore, delete snapshot, then delete the machine.
9. (Optional) Wire Atlas: Settings → Integrations → Atlas (`docs/ATLAS.md`); mint `product.service.kryton` JWT.

## Explicitly out of GA

- Live migration / HA replicas (`liveMigration` remains false).
- Dockur snapshots.
- Microsoft media, product keys, and Windows licensing (operator-owned).
