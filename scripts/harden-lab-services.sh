#!/usr/bin/env bash
# Install hardened systemd units (apikey auth) for dockur + kubevirt lab APIs on this host.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-${0}}")" 2>/dev/null && pwd || true)"
PUBLIC_HOST="${KRYTON_LAB_PUBLIC_HOST:-$(hostname -I 2>/dev/null | awk '{print $1}')}"
DOCKUR_PORT="${KRYTON_DOCKUR_PORT:-7088}"
KV_PORT="${KRYTON_KV_PORT:-9088}"
USER_NAME="${KRYTON_LAB_USER:-$(id -un)}"
HOME_DIR="${KRYTON_LAB_HOME:-${HOME}}"
KEYS_DIR="${KRYTON_KEYS_DIR:-${HOME_DIR}/.kryton}"
KEYS_FILE="${KRYTON_API_KEYS_FILE:-${KEYS_DIR}/keys.json}"
DEPLOY_DIR="${KRYTON_PROJECT_ROOT:-${HOME_DIR}/.deployments/kryton}"
ENSURE_KEYS="${SCRIPT_DIR}/ensure-api-keys.sh"
[ -x "${ENSURE_KEYS}" ] || ENSURE_KEYS="${DEPLOY_DIR}/scripts/ensure-api-keys.sh"

SUDO=""; [ "$(id -u)" -ne 0 ] && SUDO="sudo"

export KRYTON_KEYS_DIR="${KEYS_DIR}"
"${ENSURE_KEYS}"

mkdir -p "${KEYS_DIR}"
touch "${HOME_DIR}/.kryton/events-dockur.jsonl" "${HOME_DIR}/.kryton/events-kubevirt.jsonl" 2>/dev/null || true
chown "${USER_NAME}:${USER_NAME}" "${KEYS_DIR}" "${HOME_DIR}/.kryton/events-dockur.jsonl" "${HOME_DIR}/.kryton/events-kubevirt.jsonl" 2>/dev/null || \
  ${SUDO} chown "${USER_NAME}:${USER_NAME}" "${KEYS_DIR}" "${HOME_DIR}/.kryton/events-dockur.jsonl" "${HOME_DIR}/.kryton/events-kubevirt.jsonl"

TOKEN="$(cat "${KEYS_DIR}/lab.token")"

install_dockur() {
  ${SUDO} tee /etc/systemd/system/kryton-dockur.service >/dev/null <<UNIT
[Unit]
Description=Kryton dockur Windows lab provider
After=network-online.target docker.service
Wants=network-online.target

[Service]
Type=simple
User=${USER_NAME}
Group=docker
Environment=KRYTON_PROVIDER=dockur
Environment=KRYTON_AUTH_MODE=apikey
Environment=KRYTON_API_KEYS_FILE=${KEYS_FILE}
Environment=KRYTON_LAB_AUTO_AUTH=true
Environment=KRYTON_LAB_TOKEN_FILE=${KEYS_DIR}/lab.token
Environment=KRYTON_ALLOW_INSECURE=true
Environment=KRYTON_ADDR=:${DOCKUR_PORT}
Environment=KRYTON_DOCKUR_RUNTIME=docker
Environment=KRYTON_DOCKUR_PUBLIC_HOST=${PUBLIC_HOST}
Environment=KRYTON_DOCKUR_DATA_DIR=${HOME_DIR}/.kryton/dockur
Environment=KRYTON_DOCKUR_HTTP_BASE=18006
Environment=KRYTON_DOCKUR_RDP_BASE=13389
Environment=KRYTON_PROJECTS=default
Environment=KRYTON_DEFAULT_PROJECT=default
Environment=KRYTON_EVENTS_FILE=${HOME_DIR}/.kryton/events-dockur.jsonl
Environment=KRYTON_PROJECT_ROOT=${DEPLOY_DIR}
Environment=KRYTON_CORS_ORIGINS=*
ExecStart=/usr/local/bin/krytond
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
UNIT
}

install_kubevirt() {
  ${SUDO} tee /etc/systemd/system/kryton-kubevirt.service >/dev/null <<UNIT
[Unit]
Description=Kryton KubeVirt control plane
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${USER_NAME}
Environment=KRYTON_PROVIDER=kubevirt
Environment=KRYTON_AUTH_MODE=apikey
Environment=KRYTON_API_KEYS_FILE=${KEYS_FILE}
Environment=KRYTON_LAB_AUTO_AUTH=true
Environment=KRYTON_LAB_TOKEN_FILE=${KEYS_DIR}/lab.token
Environment=KRYTON_ALLOW_INSECURE=true
Environment=KRYTON_ADDR=:${KV_PORT}
Environment=KRYTON_IMAGE_NAMESPACE=kryton-images
Environment=KRYTON_STORAGE_CLASS=rook-ceph-block
Environment=KRYTON_PROJECTS=default
Environment=KRYTON_DEFAULT_PROJECT=default
Environment=KRYTON_KUBECONFIG=${HOME_DIR}/.kube/config
Environment=KRYTON_PROJECT_ROOT=${DEPLOY_DIR}
Environment=KRYTON_CORS_ORIGINS=*
Environment=KRYTON_EVENTS_FILE=${HOME_DIR}/.kryton/events-kubevirt.jsonl
ExecStart=/usr/local/bin/krytond
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
UNIT
}

install_dockur
install_kubevirt
${SUDO} systemctl daemon-reload
${SUDO} systemctl enable kryton-dockur.service kryton-kubevirt.service
${SUDO} systemctl restart kryton-dockur.service kryton-kubevirt.service
sleep 2

check() {
  local port="$1" name="$2"
  if curl -fsS -H "Authorization: Bearer ${TOKEN}" "http://127.0.0.1:${port}/readyz" >/dev/null; then
    echo "✓ ${name} :${port} ready (apikey)"
  else
    echo "✗ ${name} :${port} failed — journalctl -u kryton-${name}.service -n 30"
    return 1
  fi
}

check "${DOCKUR_PORT}" dockur
check "${KV_PORT}" kubevirt || true

DOC="$(curl -fsS -H "Authorization: Bearer ${TOKEN}" "http://127.0.0.1:${DOCKUR_PORT}/api/v1/doctor")"
AUTH="$(echo "${DOC}" | jq -r '.findings[] | select(.check=="auth") | .status')"
echo "dockur doctor auth: ${AUTH}"

cat <<EOF

Lab hardened (apikey auth).

  Dockur UI:  http://${PUBLIC_HOST}:${DOCKUR_PORT}/
  KubeVirt:   http://${PUBLIC_HOST}:${KV_PORT}/

  Token file: ${KEYS_DIR}/lab.token
  Keys file:  ${KEYS_FILE}

  export KRYTON_TOKEN=\$(cat ${KEYS_DIR}/lab.token)
  export KRYTON_URL=http://${PUBLIC_HOST}:${DOCKUR_PORT}

Paste the token in the UI auth dialog on first visit (skip when KRYTON_LAB_AUTO_AUTH=true — UI auto-connects).
EOF
