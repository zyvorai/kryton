#!/usr/bin/env bash
# Full KubeVirt production path: golden qcow2 → CDI DataSource → Kryton API → Windows VM.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

BUILD_GOLDEN=false
CUSTOMER_HELM=false
HTTP_MODE=false
SKIP_CREATE=false
IMAGE=""
IMAGE_ID="${KRYTON_IMAGE_ID:-windows-11-enterprise}"
VERSION="${VERSION:-11e}"

usage() {
  cat <<EOF
Production KubeVirt setup for Kryton (golden image + CDI + API + VM).

Usage:
  KRYTON_WINDOWS_IMAGE=/path/to/win11.qcow2 $0
  $0 --build-golden                         # build sysprepped qcow2 first (dockur, ~45–90m)
  $0 --build-golden --skip-create           # bootstrap only
  $0 --customer-helm --build-golden         # Helm values-customer.yaml profile

Examples:
  $0 --build-golden
  KRYTON_WINDOWS_IMAGE=./out/windows-11e-golden.qcow2 $0
  make setup-kubevirt-production BUILD=1

See docs/GOLDEN-IMAGES.md and docs/CUSTOMER.md.
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    --build-golden) BUILD_GOLDEN=true; shift ;;
    --customer-helm) CUSTOMER_HELM=true; shift ;;
    --http) HTTP_MODE=true; shift ;;
    --image) IMAGE="$2"; shift 2 ;;
    --id) IMAGE_ID="$2"; shift 2 ;;
    --skip-create) SKIP_CREATE=true; shift ;;
    --version) VERSION="$2"; shift 2 ;;
    *) echo "unknown arg: $1" >&2; usage; exit 1 ;;
  esac
done

GOLDEN_OUT="${KRYTON_WINDOWS_IMAGE:-${PROJECT_DIR}/out/windows-${VERSION}-golden.qcow2}"

if [ -n "${IMAGE}" ]; then
  GOLDEN_OUT="${IMAGE}"
fi

if [ ! -f "${GOLDEN_OUT}" ]; then
  if [ "${BUILD_GOLDEN}" = true ]; then
    echo "=== Building golden Windows image (VERSION=${VERSION}) ==="
    VERSION="${VERSION}" KRYTON_IMAGE_ID="${IMAGE_ID}" \
      "${SCRIPT_DIR}/build-golden-image.sh" --auto --version "${VERSION}" --image-id "${IMAGE_ID}"
    if [ ! -f "${GOLDEN_OUT}" ]; then
      echo "error: golden build did not produce ${GOLDEN_OUT}" >&2
      exit 1
    fi
  elif [ -z "${KRYTON_IMAGE_URL:-}" ] && [ "${HTTP_MODE}" != true ]; then
    echo "error: no golden image at ${GOLDEN_OUT}" >&2
    echo "  provide KRYTON_WINDOWS_IMAGE, KRYTON_IMAGE_URL, or pass --build-golden" >&2
    exit 1
  fi
fi

SETUP_ARGS=(--id "${IMAGE_ID}")
if [ "${SKIP_CREATE}" = true ]; then
  SETUP_ARGS+=(--skip-create)
fi
if [ "${HTTP_MODE}" = true ]; then
  SETUP_ARGS+=(--http)
fi
if [ -n "${IMAGE}" ] || [ -f "${GOLDEN_OUT}" ]; then
  SETUP_ARGS+=(--image "${GOLDEN_OUT}")
fi

if [ "${CUSTOMER_HELM}" = true ]; then
  export KRYTON_HELM_VALUES="${PROJECT_DIR}/deploy/helm/kryton/values-customer.yaml"
  SETUP_ARGS+=(--helm --customer)
fi

echo "=== Kryton KubeVirt production setup ==="
echo "  golden: ${GOLDEN_OUT}"
echo "  image id: ${IMAGE_ID}"

export KRYTON_WINDOWS_IMAGE="${GOLDEN_OUT}"
export KRYTON_IMAGE_ID="${IMAGE_ID}"
"${SCRIPT_DIR}/setup-kubevirt.sh" "${SETUP_ARGS[@]}"
