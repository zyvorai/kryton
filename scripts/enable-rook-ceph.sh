#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Install Rook Ceph RBD as the KubeVirt VM disk + snapshot backend.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
ROOK_VERSION="${ROOK_VERSION:-v1.16.7}"
CEPH_IMAGE="${CEPH_IMAGE:-quay.io/ceph/ceph:v19.2.2}"
MODE="" # lab | devices | device | pool-only
DEVICE=""
WIPE_DEVICE=false

usage() {
  cat <<EOF
Install Rook Ceph so Kryton VM disks and VirtualMachineSnapshots use RBD.

Usage:
  $0 --lab                              Single-node directory OSD (not for production)
  $0 --devices                          CephCluster with useAllDevices=true
  $0 --device /dev/sdb1 [--wipe-device] Explicit OSD device/partition (lab/single-node)
  $0 --pool-only                        Operator/cluster already exist; apply pool + SC + VSC

Environment:
  ROOK_VERSION   Rook example tag (default: ${ROOK_VERSION})
  CEPH_IMAGE     Ceph container image (default: ${CEPH_IMAGE})

Lab note (this host): OS is /dev/sda; k3s data is /dev/sdb2 (/var/lib/rancher).
Use only the Ceph partition, e.g. --device /dev/sdb1. Never pass whole /dev/sdb.

Then:
  export KRYTON_STORAGE_CLASS=rook-ceph-block
  helm upgrade ... -f deploy/helm/kryton/values-rook-ceph.yaml

Docs: docs/STORAGE.md
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    --lab) MODE=lab; shift ;;
    --devices) MODE=devices; shift ;;
    --device) MODE=device; DEVICE="$2"; shift 2 ;;
    --wipe-device) WIPE_DEVICE=true; shift ;;
    --pool-only) MODE=pool-only; shift ;;
    *) echo "unknown arg: $1" >&2; usage; exit 1 ;;
  esac
done
if [ -z "${MODE}" ]; then
  echo "specify --lab, --devices, --device PATH, or --pool-only" >&2
  usage
  exit 1
fi
if [ "${MODE}" = device ] && [ -z "${DEVICE}" ]; then
  echo "--device requires a path like /dev/sdb1" >&2
  exit 1
fi

need() { command -v "$1" >/dev/null || { echo "missing: $1" >&2; exit 1; }; }
need kubectl
need jq

BASE="https://raw.githubusercontent.com/rook/rook/${ROOK_VERSION}/deploy/examples"

echo "=== Rook Ceph for Kryton VM disks/snapshots (${MODE}) ==="
kubectl cluster-info >/dev/null

device_basename() {
  basename "$1"
}

assert_safe_device() {
  local d="$1"
  case "$d" in
    /dev/sda|/dev/sda[0-9]*|/dev/nvme0n1|/dev/nvme0n1p*)
      echo "refusing OS disk path: $d" >&2
      exit 1
      ;;
    /dev/sdb)
      echo "refusing whole /dev/sdb (sdb2 holds k3s). Use /dev/sdb1." >&2
      exit 1
      ;;
    /dev/sdb2)
      echo "refusing /dev/sdb2 (mounted as /var/lib/rancher / k3s-data)." >&2
      exit 1
      ;;
  esac
}

install_operator() {
  echo "→ Rook operator ${ROOK_VERSION}"
  kubectl apply -f "${BASE}/crds.yaml"
  kubectl apply -f "${BASE}/common.yaml"
  kubectl apply -f "${BASE}/operator.yaml"
  kubectl -n rook-ceph rollout status deploy/rook-ceph-operator --timeout=300s
}

apply_lab_cluster() {
  local node
  node="$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}')"
  echo "→ CephCluster lab (directory OSD on ${node})"
  kubectl apply -f - <<EOF
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: ${CEPH_IMAGE}
  dataDirHostPath: /var/lib/rook
  mon:
    count: 1
    allowMultiplePerNode: true
  mgr:
    count: 1
    allowMultiplePerNode: true
  dashboard:
    enabled: false
  crashCollector:
    disable: true
  storage:
    useAllNodes: false
    useAllDevices: false
    nodes:
      - name: ${node}
        directories:
          - path: /var/lib/rook-osd
EOF
}

apply_devices_cluster() {
  echo "→ CephCluster useAllDevices=true (all unused disks — confirm this is not the OS disk)"
  kubectl apply -f - <<EOF
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: ${CEPH_IMAGE}
  dataDirHostPath: /var/lib/rook
  mon:
    count: 3
    allowMultiplePerNode: false
  dashboard:
    enabled: false
  storage:
    useAllNodes: true
    useAllDevices: true
EOF
}

apply_device_cluster() {
  local node name
  assert_safe_device "${DEVICE}"
  node="$(kubectl get nodes -o jsonpath='{.items[0].metadata.name}')"
  name="$(device_basename "${DEVICE}")"
  echo "→ CephCluster single-node using device ${DEVICE} (${name}) on ${node}"
  if [ "${WIPE_DEVICE}" = true ]; then
    echo "→ Wiping leftover signatures on ${DEVICE}"
    if [ "$(id -u)" -eq 0 ]; then
      wipefs -a "${DEVICE}" || true
      sgdisk --zap-all "${DEVICE}" 2>/dev/null || true
    elif command -v sudo >/dev/null; then
      sudo -n wipefs -a "${DEVICE}" || true
      sudo -n sgdisk --zap-all "${DEVICE}" 2>/dev/null || true
    else
      echo "cannot wipe ${DEVICE}: need root/sudo" >&2
      exit 1
    fi
  fi
  kubectl apply -f - <<EOF
apiVersion: ceph.rook.io/v1
kind: CephCluster
metadata:
  name: rook-ceph
  namespace: rook-ceph
spec:
  cephVersion:
    image: ${CEPH_IMAGE}
  dataDirHostPath: /var/lib/rook
  mon:
    count: 1
    allowMultiplePerNode: true
  mgr:
    count: 1
    allowMultiplePerNode: true
  dashboard:
    enabled: false
  crashCollector:
    disable: true
  storage:
    useAllNodes: false
    useAllDevices: false
    nodes:
      - name: ${node}
        devices:
          - name: ${name}
EOF
}

wait_cluster() {
  echo "→ Waiting for CephCluster Ready (up to 15 min)"
  kubectl -n rook-ceph wait cephcluster/rook-ceph --for=jsonpath='{.status.phase}'=Ready --timeout=900s
}

apply_block() {
  local pool="${PROJECT_DIR}/deploy/rook-ceph/block-pool.yaml"
  echo "→ Block pool + StorageClass + VolumeSnapshotClass"
  if [ "${MODE}" = lab ] || [ "${MODE}" = device ]; then
    kubectl apply -f - <<'EOF'
apiVersion: ceph.rook.io/v1
kind: CephBlockPool
metadata:
  name: replicapool
  namespace: rook-ceph
spec:
  failureDomain: osd
  replicated:
    size: 1
    requireSafeReplicaSize: false
EOF
  else
    kubectl apply -f "${pool}"
  fi
  kubectl apply -f "${PROJECT_DIR}/deploy/rook-ceph/storageclass.yaml"
  kubectl apply -f "${PROJECT_DIR}/deploy/rook-ceph/volumesnapshotclass.yaml"
  echo "→ CDI StorageProfile snapshot class (when CDI has seen the StorageClass)"
  for _ in $(seq 1 30); do
    if kubectl get storageprofile rook-ceph-block >/dev/null 2>&1; then
      kubectl patch storageprofile rook-ceph-block --type merge -p '{"spec":{"snapshotClass":"rook-ceph-block"}}' || true
      break
    fi
    sleep 2
  done
}

case "${MODE}" in
  pool-only)
    kubectl get ns rook-ceph >/dev/null
    kubectl -n rook-ceph get cephcluster rook-ceph >/dev/null
    ;;
  lab)
    install_operator
    apply_lab_cluster
    wait_cluster
    ;;
  devices)
    install_operator
    apply_devices_cluster
    wait_cluster
    ;;
  device)
    install_operator
    apply_device_cluster
    wait_cluster
    ;;
esac

apply_block

echo ""
echo "✓ Rook Ceph RBD is wired for Kryton"
kubectl get storageclass rook-ceph-block
kubectl get volumesnapshotclass rook-ceph-block
echo ""
echo "  export KRYTON_STORAGE_CLASS=rook-ceph-block"
echo "  helm upgrade --install kryton ./deploy/helm/kryton -n kryton -f deploy/helm/kryton/values-rook-ceph.yaml"
echo "See docs/STORAGE.md"
