#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# One-shot KubeVirt + Kryton API setup and Windows VM smoke test.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

MODE="host"
PORT="${KRYTON_PORT:-9088}"
IMAGE=""
IMAGE_ID="windows-11-enterprise"
VM_NAME="win11-k8s-01"
SKIP_CREATE=false
SKIP_BOOTSTRAP=false

usage() {
  cat <<EOF
Automated KubeVirt Windows setup for Kryton.

Usage:
  KRYTON_WINDOWS_IMAGE=/path/to/win11.qcow2 $0
  $0 --image /path/to/win11.qcow2 [--helm] [--port 9088] [--skip-create]

Modes:
  (default)   Build krytond on this host + systemd unit (uses kubeconfig)
  --helm      Deploy Kryton in-cluster via Helm (values-lab.yaml, NodePort)

Steps:
  1. Verify kubectl + KubeVirt + CDI
  2. Bootstrap CDI DataSource (scripts/bootstrap-kubevirt-images.sh)
  3. Install Kryton API (host systemd or Helm)
  4. POST /api/v1/machines (windows-11-enterprise) and poll status

Environment:
  KRYTON_WINDOWS_IMAGE   Golden Windows qcow2 for bootstrap
  KRYTON_PORT            API listen port (default: 9088 host, auto-increments if busy)
  KRYTON_URL             Override API base URL for smoke test
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    --helm) MODE=helm; shift ;;
    --image) IMAGE="$2"; shift 2 ;;
    --port) PORT="$2"; shift 2 ;;
    --name) VM_NAME="$2"; shift 2 ;;
    --skip-create) SKIP_CREATE=true; shift ;;
    --skip-bootstrap) SKIP_BOOTSTRAP=true; shift ;;
    *) echo "unknown arg: $1" >&2; usage; exit 1 ;;
  esac
done

need() { command -v "$1" >/dev/null || { echo "missing: $1" >&2; exit 1; }; }
need kubectl
need curl
need jq

pick_port() {
  local p="${1}"
  if command -v ss >/dev/null; then
    while ss -tln | awk '{print $4}' | grep -q ":${p}$"; do
      p=$((p + 1))
    done
  fi
  echo "${p}"
}
PORT="$(pick_port "${PORT}")"

echo "=== Kryton KubeVirt setup (API port ${PORT}) ==="

kubectl cluster-info >/dev/null
kubectl get crd virtualmachines.kubevirt.io datasources.cdi.kubevirt.io >/dev/null

if [ "${SKIP_BOOTSTRAP}" = false ]; then
  if [ -n "${IMAGE}" ]; then
    export KRYTON_WINDOWS_IMAGE="${IMAGE}"
  fi
  "${SCRIPT_DIR}/bootstrap-kubevirt-images.sh" --id "${IMAGE_ID}"
fi

API_URL=""
if [ "${MODE}" = helm ]; then
  need helm
  need docker
  echo "→ Building container image"
  docker build -t kryton:local "${PROJECT_DIR}"
  if command -v k3s >/dev/null; then
    docker save kryton:local | sudo k3s ctr images import - >/dev/null
  fi
  echo "→ Installing Helm release"
  helm upgrade --install kryton "${PROJECT_DIR}/deploy/helm/kryton" \
    -n kryton --create-namespace \
    -f "${PROJECT_DIR}/deploy/helm/kryton/values-lab.yaml" \
    --set image.repository=kryton --set image.tag=local
  kubectl -n kryton rollout status deployment/kryton --timeout=180s
  NODE_IP="$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')"
  NODE_PORT="$(kubectl -n kryton get svc kryton -o jsonpath='{.spec.ports[0].nodePort}')"
  API_URL="http://${NODE_IP}:${NODE_PORT}"
else
  echo "→ Building binaries"
  make -C "${PROJECT_DIR}" build
  sudo install -m755 "${PROJECT_DIR}/bin/krytond" "${PROJECT_DIR}/bin/krytonctl" /usr/local/bin/
  sudo tee /etc/systemd/system/kryton-kubevirt.service >/dev/null <<UNIT
[Unit]
Description=Kryton KubeVirt control plane
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
Environment=KRYTON_PROVIDER=kubevirt
Environment=KRYTON_AUTH_MODE=disabled
Environment=KRYTON_ALLOW_INSECURE=true
Environment=KRYTON_ADDR=:${PORT}
Environment=KRYTON_IMAGE_NAMESPACE=kryton-images
Environment=KRYTON_PROJECTS=default
Environment=KRYTON_DEFAULT_PROJECT=default
Environment=KRYTON_KUBECONFIG=${HOME}/.kube/config
Environment=KRYTON_EVENTS_FILE=${HOME}/.kryton/events-kubevirt.jsonl
ExecStart=/usr/local/bin/krytond
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
UNIT
  sudo systemctl daemon-reload
  sudo systemctl enable --now kryton-kubevirt.service
  sleep 2
  HOST_IP="$(hostname -I 2>/dev/null | awk '{print $1}')"
  API_URL="http://127.0.0.1:${PORT}"
  echo "→ API on host :${PORT} (public: http://${HOST_IP}:${PORT})"
fi

API_URL="${KRYTON_URL:-${API_URL}}"
echo "→ Waiting for ${API_URL}/readyz"
for _ in $(seq 1 30); do
  if curl -fsS "${API_URL}/readyz" >/dev/null 2>&1; then break; fi
  sleep 2
done
curl -fsS "${API_URL}/readyz" >/dev/null

echo "→ Doctor"
curl -sS "${API_URL}/api/v1/doctor" | jq .

if [ "${SKIP_CREATE}" = true ]; then
  echo "✓ Setup complete (skipped VM create)"
  echo "  API: ${API_URL}"
  exit 0
fi

echo "→ Creating Windows 11 VM: ${VM_NAME}"
CREATE="$(curl -fsS -X POST "${API_URL}/api/v1/machines" \
  -H 'Content-Type: application/json' \
  -d "{\"project\":\"default\",\"name\":\"${VM_NAME}\",\"image\":\"${IMAGE_ID}\",\"compute\":{\"cpu\":4,\"memoryMiB\":8192},\"disk\":{\"sizeGiB\":96}}")"
echo "${CREATE}" | jq .
MID="$(echo "${CREATE}" | jq -r '.id')"

echo "→ Polling machine (up to 10 min)"
for _ in $(seq 1 60); do
  sleep 10
  BODY="$(curl -fsS "${API_URL}/api/v1/machines/${MID}?project=default")"
  STATE="$(echo "${BODY}" | jq -r '.state')"
  MSG="$(echo "${BODY}" | jq -r '.message // empty')"
  IPS="$(echo "${BODY}" | jq -r '.ipAddresses | join(", ") // empty')"
  echo "  state=${STATE} ips=${IPS} ${MSG}"
  case "${STATE}" in
    running|stopped) break ;;
    failed) echo "${BODY}" | jq .; exit 1 ;;
  esac
done

echo ""
echo "✓ Kryton KubeVirt API is live"
echo "  URL:     ${API_URL}"
echo "  UI:      ${API_URL}/"
echo "  VM id:   ${MID}"
echo "  VM name: ${VM_NAME}"
echo ""
echo "kubectl -n default get vm,vmi,pvc"
echo "KRYTON_URL=${API_URL} krytonctl get ${MID}"
