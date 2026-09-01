# Kryton VM disks and snapshots

KubeVirt `VirtualMachineSnapshot` only captures **PVC data** when the disk StorageClass is a **CSI driver with a matching VolumeSnapshotClass**. `rancher.io/local-path` cannot do that.

Kryton does not provision storage itself. It stamps `disk.storageClass` (or `KRYTON_STORAGE_CLASS`) onto the VM DataVolume. Golden images should use the **same** class (`KRYTON_STORAGE_CLASS` in `bootstrap-kubevirt-images.sh`).

## Backends

| Backend | Script | StorageClass | VolumeSnapshotClass | When to use |
|---------|--------|--------------|---------------------|-------------|
| **Rook Ceph RBD** | `scripts/enable-rook-ceph.sh` | `rook-ceph-block` | `rook-ceph-block` (`rook-ceph.rbd.csi.ceph.com`) | Production VM disks and snapshots (replication, clones, expansion) |
| **Longhorn** | `scripts/enable-kubevirt-snapshots.sh --storage longhorn` | `longhorn` | `longhorn` (`driver.longhorn.io`) | Single-node / lab CSI when you do not have Ceph devices |
| Cluster default | — | empty | none unless you add one | Avoid for Kryton VMs if default is `local-path` |

Both need the KubeVirt **Snapshot** feature gate and the CSI external-snapshotter (the snapshot scripts install those).

## Rook Ceph (recommended production)

RBD with `imageFeatures: layering` is what KubeVirt and CDI use for volume snapshots and clones.

```bash
# Feature gate + CSI snapshotter, then Rook operator, pool, StorageClass, VolumeSnapshotClass
./scripts/enable-kubevirt-snapshots.sh --storage rook-ceph --device /dev/sdb1 --wipe-device
# or, if the snapshot stack is already on:
./scripts/enable-rook-ceph.sh --device /dev/sdb1 --wipe-device

export KRYTON_STORAGE_CLASS=rook-ceph-block
# Helm: -f deploy/helm/kryton/values-rook-ceph.yaml
```

| Mode | Flag | Cluster shape |
|------|------|----------------|
| Explicit device | `--device /dev/sdb1` | Single-node OSD on that partition (this lab: Ceph on `sdb1`, k3s on `sdb2`) |
| Lab / single node | `--lab` | 1 mon, 1 replica pool, directory OSD (`/var/lib/rook-osd`) |
| Production | `--devices` | `useAllDevices: true` — only when **non-OS disks** are dedicated to Ceph |
| Existing cluster | `--pool-only` | Operator already running; only apply pool + StorageClass + VolumeSnapshotClass |

On the Zyvor lab host: **never** pass whole `/dev/sdb` — `sdb2` is `/var/lib/rancher` (k3s). Use `/dev/sdb1` only. `--wipe-device` clears leftover `ceph_bluestore` signatures before Rook claims the partition.

Do not run `--devices` on a laptop or a node whose only disk is the OS volume.

Production checklist:

- Three (or more) nodes, or enough OSDs for `replicated.size: 3` and `failureDomain: host`
- Raw devices or partitioned OSDs, not the OS disk
- `imageFeatures: layering` on the block StorageClass (shipped in `deploy/rook-ceph/`)
- Point Kryton at `rook-ceph-block` so new VMs and golden PVCs land on RBD

## Longhorn (lab CSI)

```bash
./scripts/enable-kubevirt-snapshots.sh --storage longhorn
export KRYTON_STORAGE_CLASS=longhorn
```

Use replica count 1 on a single node. Do not stack large Longhorn *and* Rook directory OSDs on the same full disk.

## Create and snapshot

From the UI (**Settings → Cluster storage**), install Longhorn or Rook Ceph (when `krytond` runs on the cluster node with `scripts/` present), then pick the default StorageClass for new machines. Create machine also has an optional per-VM storage class override.

```bash
# API: install Longhorn (async job, logs at GET /api/v1/storage/setup)
curl -X POST http://127.0.0.1:9088/api/v1/storage/setup \
  -H 'Content-Type: application/json' \
  -d '{"backend":"longhorn","setDefault":true}'

# Rook on a dedicated partition (lab)
curl -X POST http://127.0.0.1:9088/api/v1/storage/setup \
  -H 'Content-Type: application/json' \
  -d '{"backend":"rook-ceph","rookMode":"device","device":"/dev/sdb1","wipeDevice":true,"setDefault":true}'
```

```bash
# CLI equivalent
krytonctl storage
krytonctl set-storage rook-ceph-block

export KRYTON_PROVIDER=kubevirt
# Optional env default (UI/CLI override is persisted under ~/.kryton/storage.json)
export KRYTON_STORAGE_CLASS=rook-ceph-block

krytonctl create finance-win01 --image windows-11-enterprise
```

Existing VMs whose PVCs are already on `local-path` cannot grow a disk snapshot. Recreate them on RBD or Longhorn.

For suite-wide storage ownership and discovery via Atlas, see [ATLAS.md](ATLAS.md).

## CDI StorageProfile

After the StorageClass exists, CDI creates a `StorageProfile`. The Rook script sets `spec.snapshotClass: rook-ceph-block` when that profile appears so clones/snapshots pick the RBD snapshot class.
