#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Bootstrap CDI DataSources for Kryton Windows images on KubeVirt.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

IMAGE_NS="${KRYTON_IMAGE_NAMESPACE:-kryton-images}"
STORAGE_CLASS="${KRYTON_STORAGE_CLASS:-}"
IMAGE_ID="${KRYTON_IMAGE_ID:-windows-11-enterprise}"
DISK_GIB="${KRYTON_DISK_GIB:-96}"
SRC="${KRYTON_WINDOWS_IMAGE:-}"

usage() {
  cat <<EOF
Bootstrap Kryton KubeVirt Windows golden images (CDI DataSource + PVC).

Usage:
  KRYTON_WINDOWS_IMAGE=/path/to/windows11.qcow2 $0
  $0 --image /path/to/windows11.qcow2 [--id windows-11-enterprise]

Environment:
  KRYTON_WINDOWS_IMAGE   Sysprepped Windows qcow2 (required unless DataSource exists)
  KRYTON_IMAGE_ID        Catalog image ID / DataSource name (default: windows-11-enterprise)
  KRYTON_IMAGE_NAMESPACE CDI namespace (default: kryton-images)
  KRYTON_DISK_GIB        Golden PVC size (default: 96)
  KRYTON_STORAGE_CLASS   Override storage class (default: cluster default)
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    --image) SRC="$2"; shift 2 ;;
    --id) IMAGE_ID="$2"; shift 2 ;;
    *) echo "unknown arg: $1" >&2; usage; exit 1 ;;
  esac
done

need() { command -v "$1" >/dev/null || { echo "missing required command: $1" >&2; exit 1; }; }
need kubectl

echo "→ Ensuring Kryton project namespaces"
kubectl apply -f "${PROJECT_DIR}/deploy/kubevirt/namespaces.yaml"
for ns in default; do
  kubectl get namespace "${ns}" >/dev/null 2>&1 || kubectl create namespace "${ns}" --dry-run=client -o yaml | kubectl apply -f -
done

if kubectl -n "${IMAGE_NS}" get datasource "${IMAGE_ID}" >/dev/null 2>&1; then
  echo "✓ DataSource ${IMAGE_NS}/${IMAGE_ID} already exists"
  exit 0
fi

if [ -z "${SRC}" ]; then
  echo "error: set KRYTON_WINDOWS_IMAGE to a sysprepped Windows qcow2" >&2
  echo "  example: KRYTON_WINDOWS_IMAGE=./windows11.qcow2 $0" >&2
  exit 1
fi
if [ ! -f "${SRC}" ]; then
  echo "error: image not found: ${SRC}" >&2
  exit 1
fi

need qemu-img
if command -v virtctl >/dev/null; then
  VIRTCTL=virtctl
elif command -v kubectl-virt >/dev/null; then
  VIRTCTL=kubectl-virt
else
  echo "error: virtctl is required for CDI upload (install KubeVirt virtctl)" >&2
  exit 1
fi

WORK="$(mktemp -d)"
trap 'rm -rf "${WORK}"' EXIT
UPLOAD_SRC="${SRC}"
if qemu-img info "${SRC}" | grep -q 'backing file:'; then
  echo "→ Flattening qcow2 overlay"
  UPLOAD_SRC="${WORK}/golden.qcow2"
  qemu-img convert -p -O qcow2 "${SRC}" "${UPLOAD_SRC}"
fi

DV="${IMAGE_ID}-golden"
SC_LINE=""
if [ -n "${STORAGE_CLASS}" ]; then
  SC_LINE="storageClassName: ${STORAGE_CLASS}"
fi

echo "→ Creating upload DataVolume ${IMAGE_NS}/${DV}"
kubectl apply -f - <<EOF
apiVersion: cdi.kubevirt.io/v1beta1
kind: DataVolume
metadata:
  name: ${DV}
  namespace: ${IMAGE_NS}
spec:
  source:
    upload: {}
  pvc:
    accessModes:
      - ReadWriteOnce
    resources:
      requests:
        storage: ${DISK_GIB}Gi
    ${SC_LINE}
EOF

echo "→ Uploading image (this may take several minutes)"
${VIRTCTL} image-upload dv "${DV}" \
  --namespace="${IMAGE_NS}" \
  --size="${DISK_GIB}Gi" \
  --image-path="${UPLOAD_SRC}" \
  --insecure \
  --wait-secs=240

echo "→ Waiting for DataVolume to succeed"
kubectl -n "${IMAGE_NS}" wait "datavolume/${DV}" --for=condition=Ready --timeout=30m

echo "→ Creating DataSource ${IMAGE_NS}/${IMAGE_ID}"
kubectl apply -f - <<EOF
apiVersion: cdi.kubevirt.io/v1beta1
kind: DataSource
metadata:
  name: ${IMAGE_ID}
  namespace: ${IMAGE_NS}
  labels:
    app.kubernetes.io/managed-by: kryton
spec:
  source:
    pvc:
      name: ${DV}
      namespace: ${IMAGE_NS}
EOF

echo "✓ Bootstrap complete: ${IMAGE_NS}/${IMAGE_ID}"
