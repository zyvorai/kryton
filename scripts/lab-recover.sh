#!/usr/bin/env bash
# Copyright 2026 Kryton contributors
# SPDX-License-Identifier: Apache-2.0
# Recover a hung lab host before long-running Kryton jobs (free ports, restart APIs).
set -euo pipefail

PUBLIC_HOST="${KRYTON_LAB_PUBLIC_HOST:-$(hostname -I 2>/dev/null | awk '{print $1}')}"
DOCKUR_PORT="${KRYTON_DOCKUR_PORT:-7088}"
KV_PORT="${KRYTON_KV_PORT:-9088}"

SUDO=""; [ "$(id -u)" -ne 0 ] && SUDO="sudo"

echo "=== Kryton lab recovery ==="

echo "→ Stop systemd units (ignore errors)"
${SUDO} systemctl stop kryton-dockur.service kryton-kubevirt.service kryton.service 2>/dev/null || true

echo "→ Kill stray krytond listeners"
for pid in $(pgrep -x krytond 2>/dev/null || true); do
  echo "  kill krytond pid=${pid}"
  kill "${pid}" 2>/dev/null || ${SUDO} kill "${pid}" 2>/dev/null || true
done
sleep 2
for pid in $(pgrep -x krytond 2>/dev/null || true); do
  kill -9 "${pid}" 2>/dev/null || ${SUDO} kill -9 "${pid}" 2>/dev/null || true
done

echo "→ Port check (${DOCKUR_PORT}, ${KV_PORT})"
if command -v ss >/dev/null; then
  ss -tlnp | grep -E ":(${DOCKUR_PORT}|${KV_PORT})\\b" || echo "  ports free"
fi

echo "→ Fix events file ownership"
mkdir -p "${HOME}/.kryton"
touch "${HOME}/.kryton/events-dockur.jsonl" "${HOME}/.kryton/events-kubevirt.jsonl" 2>/dev/null || true
chown "$(id -un):$(id -gn)" "${HOME}/.kryton/events-dockur.jsonl" "${HOME}/.kryton/events-kubevirt.jsonl" 2>/dev/null || \
  ${SUDO} chown "$(id -un):$(id -gn)" "${HOME}/.kryton/events-dockur.jsonl" "${HOME}/.kryton/events-kubevirt.jsonl" 2>/dev/null || true

echo "→ Docker sanity"
if command -v docker >/dev/null; then
  sg docker -c 'docker info >/dev/null 2>&1' && echo "  docker ok" || echo "  warn: docker not reachable"
fi

echo "→ Kubernetes sanity"
if command -v kubectl >/dev/null; then
  kubectl get nodes --request-timeout=15s | head -5 || echo "  warn: kubectl slow/failed"
  kubectl get crd virtualmachines.kubevirt.io datasources.cdi.kubevirt.io --request-timeout=15s >/dev/null \
    && echo "  kubevirt+cdi crds ok" || echo "  warn: missing KubeVirt/CDI CRDs"
fi

echo "✓ Lab recovery complete (public ${PUBLIC_HOST})"
