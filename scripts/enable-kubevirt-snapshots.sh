#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Enable KubeVirt VirtualMachineSnapshot/Restore on a cluster.
#
# rancher.io/local-path cannot take CSI volume snapshots. This script:
#   1. Turns on the KubeVirt Snapshot feature gate
#   2. Installs the CSI external-snapshotter CRDs + controller if missing
#   3. Installs a CSI backend: Longhorn (default) or Rook Ceph RBD
#   4. Creates a VolumeSnapshotClass
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SNAPSHOTTER_VERSION="${SNAPSHOTTER_VERSION:-v8.2.1}"
LONGHORN_VERSION="${LONGHORN_VERSION:-v1.9.1}"
SKIP_STORAGE=false
STORAGE="" # longhorn | rook-ceph
ROOK_MODE="${ROOK_MODE:-lab}"
ROOK_DEVICE="${ROOK_DEVICE:-}"
ROOK_WIPE=false

usage() {
  cat <<EOF
Enable KubeVirt VM snapshots (feature gate + CSI VolumeSnapshotClass).

Usage:
  $0
  $0 --skip-storage              # feature gate + snapshot-controller only
  $0 --storage longhorn          # Longhorn CSI (lab / single node)
  $0 --storage rook-ceph         # Rook Ceph RBD (see --rook-mode / --device)
  $0 --storage rook-ceph --rook-mode lab|devices|pool-only
  $0 --storage rook-ceph --device /dev/sdb1 [--wipe-device]

Environment:
  SNAPSHOTTER_VERSION  kubernetes-csi/external-snapshotter tag (default: ${SNAPSHOTTER_VERSION})
  LONGHORN_VERSION     Longhorn manifest tag (default: ${LONGHORN_VERSION})
  ROOK_MODE            lab | devices | pool-only (default: lab)
  ROOK_DEVICE          OSD path when using Rook (e.g. /dev/sdb1)

Point Kryton at the class you installed:
  export KRYTON_STORAGE_CLASS=longhorn
  export KRYTON_STORAGE_CLASS=rook-ceph-block
Docs: docs/STORAGE.md
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    --skip-storage) SKIP_STORAGE=true; shift ;;
    --rook-mode) ROOK_MODE="$2"; shift 2 ;;
    --device) ROOK_DEVICE="$2"; shift 2 ;;
    --wipe-device) ROOK_WIPE=true; shift ;;
    --storage)
      case "${2:-}" in
        longhorn) STORAGE=longhorn; shift 2 ;;
        rook-ceph) STORAGE=rook-ceph; shift 2 ;;
        *) echo "unsupported --storage $2 (longhorn|rook-ceph)" >&2; exit 1 ;;
      esac
      ;;
    *) echo "unknown arg: $1" >&2; usage; exit 1 ;;
  esac
done

need() { command -v "$1" >/dev/null || { echo "missing: $1" >&2; exit 1; }; }
need kubectl
need jq

echo "=== Enable KubeVirt snapshots ==="
kubectl cluster-info >/dev/null

enable_feature_gate() {
  local ns name json gates
  ns="$(kubectl get kubevirt -A -o jsonpath='{.items[0].metadata.namespace}' 2>/dev/null || true)"
  name="$(kubectl get kubevirt -A -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
  if [ -z "${ns}" ] || [ -z "${name}" ]; then
    echo "KubeVirt CR not found; install KubeVirt first" >&2
    exit 1
  fi
  echo "→ Snapshot feature gate on ${ns}/${name}"
  json="$(kubectl -n "${ns}" get kubevirt "${name}" -o json)"
  gates="$(echo "${json}" | jq -c '(.spec.configuration.developerConfiguration.featureGates // []) + ["Snapshot"] | unique')"
  kubectl -n "${ns}" patch kubevirt "${name}" --type merge -p "$(jq -n --argjson g "${gates}" '{spec:{configuration:{developerConfiguration:{featureGates:$g}}}}')"
}

install_snapshotter() {
  if kubectl get crd volumesnapshotclasses.snapshot.storage.k8s.io >/dev/null 2>&1; then
    echo "→ CSI VolumeSnapshot CRDs already present"
  else
    echo "→ Installing CSI VolumeSnapshot CRDs ${SNAPSHOTTER_VERSION}"
    kubectl apply -f "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${SNAPSHOTTER_VERSION}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml"
    kubectl apply -f "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${SNAPSHOTTER_VERSION}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml"
    kubectl apply -f "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${SNAPSHOTTER_VERSION}/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml"
  fi
  if kubectl -n kube-system get deploy snapshot-controller >/dev/null 2>&1; then
    echo "→ snapshot-controller already running"
  else
    echo "→ Installing snapshot-controller ${SNAPSHOTTER_VERSION}"
    kubectl apply -f "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${SNAPSHOTTER_VERSION}/deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml"
    kubectl apply -f "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${SNAPSHOTTER_VERSION}/deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml"
  fi
}

ensure_host_iscsi() {
  if [ "$(id -u)" -eq 0 ]; then
    systemctl enable --now iscsid 2>/dev/null || service iscsid start 2>/dev/null || true
    return
  fi
  if command -v sudo >/dev/null; then
    sudo -n systemctl enable --now iscsid 2>/dev/null || sudo -n service iscsid start 2>/dev/null || true
  fi
}

install_longhorn() {
  echo "→ Installing Longhorn ${LONGHORN_VERSION} (replica count 1)"
  ensure_host_iscsi
  kubectl apply -f "https://raw.githubusercontent.com/longhorn/longhorn/${LONGHORN_VERSION}/deploy/longhorn.yaml"
  echo "→ Waiting for Longhorn (up to 8 min)"
  kubectl -n longhorn-system wait --for=condition=available deploy --all --timeout=480s || true
  echo "→ Ensuring VolumeSnapshotClass longhorn"
  kubectl apply -f - <<'EOF'
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: longhorn
  annotations:
    snapshot.storage.kubernetes.io/is-default-class: "true"
driver: driver.longhorn.io
deletionPolicy: Delete
parameters:
  type: snap
EOF
  if kubectl get settings.longhorn.io -n longhorn-system default-replica-count >/dev/null 2>&1; then
    kubectl -n longhorn-system patch settings.longhorn.io default-replica-count --type merge -p '{"value":"1"}' || true
  fi
  if kubectl get settings.longhorn.io -n longhorn-system storage-minimal-available-percentage >/dev/null 2>&1; then
    kubectl -n longhorn-system patch settings.longhorn.io storage-minimal-available-percentage --type merge -p '{"value":"5"}' || true
  fi
}

enable_feature_gate
install_snapshotter

CLASS_COUNT="$(kubectl get volumesnapshotclass -o json 2>/dev/null | jq '.items | length')"
if [ "${SKIP_STORAGE}" = true ]; then
  echo "→ Skipping CSI backend (--skip-storage)"
elif [ "${STORAGE}" = "rook-ceph" ]; then
  if [ -n "${ROOK_DEVICE}" ]; then
    ROOK_ARGS=(--device "${ROOK_DEVICE}")
    if [ "${ROOK_WIPE}" = true ]; then ROOK_ARGS+=(--wipe-device); fi
    "${SCRIPT_DIR}/enable-rook-ceph.sh" "${ROOK_ARGS[@]}"
  else
    "${SCRIPT_DIR}/enable-rook-ceph.sh" "--${ROOK_MODE}"
  fi
elif [ "${STORAGE}" = "longhorn" ]; then
  if kubectl get ns longhorn-system >/dev/null 2>&1; then
    echo "→ longhorn-system exists; ensuring VolumeSnapshotClass"
    kubectl apply -f - <<'EOF'
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: longhorn
  annotations:
    snapshot.storage.kubernetes.io/is-default-class: "true"
driver: driver.longhorn.io
deletionPolicy: Delete
parameters:
  type: snap
EOF
  else
    install_longhorn
  fi
elif [ "${CLASS_COUNT}" = "0" ]; then
  install_longhorn
else
  echo "→ VolumeSnapshotClass already present"
  kubectl get volumesnapshotclass
fi

echo ""
echo "✓ KubeVirt snapshot stack"
kubectl get kubevirt -A -o jsonpath='{range .items[*]}{.metadata.namespace}/{.metadata.name} gates={.spec.configuration.developerConfiguration.featureGates}{"\n"}{end}'
kubectl get volumesnapshotclass
echo ""
echo "Create Kryton VMs on snapshot-capable disks:"
echo "  export KRYTON_STORAGE_CLASS=longhorn          # Longhorn"
echo "  export KRYTON_STORAGE_CLASS=rook-ceph-block   # Rook Ceph RBD"
echo "  # or pass disk.storageClass on POST /api/v1/machines"
echo "rancher.io/local-path disks cannot be restored from VirtualMachineSnapshot."
echo "See docs/STORAGE.md"
