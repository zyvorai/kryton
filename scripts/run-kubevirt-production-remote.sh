#!/usr/bin/env bash
# Copyright 2026 Kryton contributors
# SPDX-License-Identifier: Apache-2.0
# Run KubeVirt production setup on a remote lab host (rsync + SSH + nohup for long golden builds).
set -euo pipefail

HOST="${1:?host required — run with -h for usage}"
USER="${2:-${USER:?user required — pass as second argument or set USER}}"
REMOTE="${USER}@${HOST}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
REMOTE_DIR="${KRYTON_REMOTE_DIR:-/home/${USER}/.deployments/kryton}"
LOG="${KRYTON_PRODUCTION_LOG:-${HOME}/.kryton/kubevirt-production.log}"
BUILD_GOLDEN="${BUILD_GOLDEN:-1}"
SKIP_CREATE="${SKIP_CREATE:-0}"

SSH_OPTS=(-o BatchMode=yes -o ConnectTimeout=60 -o ServerAliveInterval=15 -o ServerAliveCountMax=4 -o GSSAPIAuthentication=no -o TCPKeepAlive=yes)

usage() {
  cat <<EOF
Usage: $0 [host] [user]

Environment:
  BUILD_GOLDEN=1|0     Build sysprepped qcow2 on remote if missing (default: 1)
  SKIP_CREATE=1        Bootstrap + API only, skip test VM
  KRYTON_REMOTE_DIR    Remote checkout path

Example:
  BUILD_GOLDEN=1 ./scripts/run-kubevirt-production-remote.sh <host> <user>
  tail -f ~/.kryton/kubevirt-production.log
EOF
}

if [ "${1:-}" = "-h" ] || [ "${1:-}" = "--help" ]; then usage; exit 0; fi

echo "→ Syncing Kryton to ${REMOTE}:${REMOTE_DIR}"
rsync -az --delete \
  --exclude .git --exclude bin --exclude node_modules --exclude out \
  "${PROJECT_DIR}/" "${REMOTE}:${REMOTE_DIR}/"

ARGS=(--id windows-11-enterprise)
if [ "${BUILD_GOLDEN}" = "1" ]; then
  ARGS+=(--build-golden)
fi
if [ "${SKIP_CREATE}" = "1" ]; then
  ARGS+=(--skip-create)
fi

REMOTE_CMD="cd '${REMOTE_DIR}' && chmod +x scripts/*.sh && ./scripts/lab-recover.sh && if [ -f out/windows-11e-golden.qcow2 ]; then nohup ./scripts/setup-kubevirt-production.sh --id windows-11-enterprise --image out/windows-11e-golden.qcow2 > '${REMOTE_DIR}/kubevirt-production.log' 2>&1 & else nohup ./scripts/setup-kubevirt-production.sh --build-golden --id windows-11-enterprise > '${REMOTE_DIR}/kubevirt-production.log' 2>&1 & fi && echo \$!"

echo "→ Starting production setup on ${REMOTE} (log: ${REMOTE_DIR}/kubevirt-production.log)"
for attempt in 1 2 3 4 5 6; do
  if ssh "${SSH_OPTS[@]}" "${REMOTE}" "${REMOTE_CMD}"; then
    echo "✓ Started on remote. Monitor:"
    echo "  ssh ${REMOTE} tail -f ${REMOTE_DIR}/kubevirt-production.log"
    exit 0
  fi
  echo "  SSH attempt ${attempt} failed, retrying in 15s..."
  sleep 15
done

echo "✗ Could not SSH to ${REMOTE}" >&2
exit 1
