#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Kryton golden-image builder — dockur/windows unattended install + Sysprep capture.
# See https://github.com/dockur/windows and docs/GOLDEN-IMAGES.md
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

VERSION="${VERSION:-11e}"
# Prefer explicit IMAGE_ID (API/parent re-exec), then KRYTON_IMAGE_ID, then default.
# Do not clobber a parent-exported IMAGE_ID with the catalog default.
IMAGE_ID="${IMAGE_ID:-${KRYTON_IMAGE_ID:-windows-11-enterprise}}"
BUILD_ID="${BUILD_ID:-}"
WORKDIR="${WORKDIR:-}"
OUT="${OUT:-}"
OEM_DIR="${OEM_DIR:-${PROJECT_DIR}/deploy/dockur/oem}"
DOCKUR_IMAGE="${DOCKUR_IMAGE:-docker.io/dockurr/windows:latest}"
# Prefer docker when present; lab hosts often only have podman.
if [[ -z "${CONTAINER_CLI:-}" ]]; then
  if command -v docker >/dev/null 2>&1; then
    CONTAINER_CLI=docker
  elif command -v podman >/dev/null 2>&1; then
    CONTAINER_CLI=podman
  else
    CONTAINER_CLI=docker
  fi
fi
RAM_SIZE="${RAM_SIZE:-8G}"
CPU_CORES="${CPU_CORES:-4}"
DISK_SIZE="${DISK_SIZE:-80G}"
PUBLIC_HOST="${PUBLIC_HOST:-127.0.0.1}"
CONSOLE_PORT="${CONSOLE_PORT:-}"
AUTO=0
# Preserve FINALIZE=1 from parent re-exec / env — never force 0 here.
FINALIZE="${FINALIZE:-0}"
NAME=""

usage() {
  cat <<EOF
Build a sysprepped Windows golden image with dockur/windows.

Usage:
  VERSION=11e $0 --auto                    # fully automated (install + sysprep + capture)
  VERSION=11e $0                           # start builder, open console, manual finalize
  VERSION=11e FINALIZE=1 $0                # capture qcow2 after Sysprep shutdown

API / UI flags:
  $0 --build-id ID --workdir DIR --version 11e --image-id windows-11-enterprise --auto

Environment (dockur/windows):
  VERSION          Windows version code (default: 11e) — see dockur FAQ table
  RAM_SIZE         Guest RAM (default: 8G)
  CPU_CORES        Guest CPUs (default: 4)
  DISK_SIZE        Disk size (default: 80G)
  DOCKUR_IMAGE     Container image (default: dockurr/windows:latest)
  OEM_DIR          Path to install.bat folder (default: deploy/dockur/oem)

Golden output:
  OUT              qcow2 path (default: ./out/windows-\$VERSION-golden.qcow2)
  KRYTON_IMAGE_ID  Kryton catalog / DataSource name
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    -h|--help) usage; exit 0 ;;
    --auto) AUTO=1; shift ;;
    --build-id) BUILD_ID="$2"; shift 2 ;;
    --workdir) WORKDIR="$2"; shift 2 ;;
    --version) VERSION="$2"; shift 2 ;;
    --image-id) IMAGE_ID="$2"; shift 2 ;;
    --oem) OEM_DIR="$2"; shift 2 ;;
    --host) PUBLIC_HOST="$2"; shift 2 ;;
    *) echo "unknown arg: $1" >&2; usage; exit 1 ;;
  esac
done

need() { command -v "$1" >/dev/null || { echo "missing: $1" >&2; exit 1; }; }

map_image_id() {
  case "${VERSION}" in
    2025) IMAGE_ID="${IMAGE_ID:-windows-server-2025}" ;;
    2022) IMAGE_ID="${IMAGE_ID:-windows-server-2022}" ;;
    11e)  IMAGE_ID="${IMAGE_ID:-windows-11-enterprise}" ;;
    11)   IMAGE_ID="${IMAGE_ID:-windows-11-pro}" ;;
    10)   IMAGE_ID="${IMAGE_ID:-windows-10-pro}" ;;
    2019) IMAGE_ID="${IMAGE_ID:-windows-server-2019}" ;;
    *)    IMAGE_ID="${IMAGE_ID:-windows-server-${VERSION}}" ;;
  esac
}
map_image_id

if [ -z "${BUILD_ID}" ]; then
  BUILD_ID="manual-${VERSION}-$(date +%s)"
fi
if [ -z "${WORKDIR}" ]; then
  WORKDIR="${PROJECT_DIR}/.kryton/golden/${BUILD_ID}"
fi
if [ -z "${OUT}" ]; then
  OUT="${PROJECT_DIR}/out/windows-${VERSION}-golden.qcow2"
fi
NAME="kryton-golden-${BUILD_ID}"
STATUS_FILE="${WORKDIR}/status.json"
JOB_LOG="${WORKDIR}/job.log"
STORAGE="${WORKDIR}/storage"
mkdir -p "${WORKDIR}" "${STORAGE}" "$(dirname "${OUT}")"

log_line() {
  local level="$1" msg="$2"
  local prefix="[INFO]"
  case "${level}" in ok) prefix="[OK]" ;; err) prefix="[ERR]" ;; warn) prefix="[WARN]" ;; esac
  printf '%s %s\n' "$(date -u +"%Y-%m-%dT%H:%M:%SZ") ${prefix}" "${msg}" >> "${JOB_LOG}"
}

write_status() {
  local state="$1" phase="$2" progress="$3" message="$4"
  local console="${5:-}"
  local output="${6:-}"
  local err="${7:-}"
  local sha="${8:-}"
  local now
  now="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  local started="${STARTED_AT:-$now}"
  mkdir -p "${WORKDIR}"
  cat >"${STATUS_FILE}" <<JSON
{
  "id": "${BUILD_ID}",
  "version": "${VERSION}",
  "imageId": "${IMAGE_ID}",
  "state": "${state}",
  "phase": "${phase}",
  "progressPercent": ${progress},
  "message": "${message}",
  "consoleUrl": "${console}",
  "outputPath": "${output}",
  "sha256": "${sha}",
  "startedAt": "${started}",
  "updatedAt": "${now}",
  "error": "${err}"
}
JSON
  log_line info "${message}"
}

pick_port() {
  local p="${1}"
  if command -v ss >/dev/null; then
    while ss -tln 2>/dev/null | awk '{print $4}' | grep -q ":${p}$"; do
      p=$((p + 1))
    done
  fi
  echo "${p}"
}

if [[ "${FINALIZE:-0}" == "1" ]]; then
  need qemu-img
  NAME="${NAME:-kryton-golden-${BUILD_ID}}"
  write_status "capturing" "convert" 88 "Stopping builder and capturing qcow2" "" "${OUT}"
  ${CONTAINER_CLI} stop -t 180 "${NAME}" >/dev/null 2>&1 || true

  DISK="${DISK:-}"
  if [[ -z "${DISK}" ]]; then
    DISK=$(find "${STORAGE}" -maxdepth 3 -type f \( -name '*.img' -o -name '*.qcow2' -o -name '*.raw' \) -size +1G 2>/dev/null | sort | head -n1 || true)
  fi
  [[ -n "${DISK}" ]] || { write_status "failed" "convert" 0 "No Windows disk found under ${STORAGE}" "" "" "disk not found"; exit 1; }

  TMP="${OUT}.tmp"
  rm -f "${TMP}"
  qemu-img convert -p -O qcow2 "${DISK}" "${TMP}"
  qemu-img check "${TMP}"
  mv "${TMP}" "${OUT}"
  SHA="$(sha256sum "${OUT}" | awk '{print $1}')"
  echo "${SHA}" >"${OUT}.sha256"
  write_status "ready" "complete" 100 "Golden image ready" "" "${OUT}" "" "${SHA}"
  echo "✓ Golden image ready: ${OUT}"
  echo "  Next: KRYTON_WINDOWS_IMAGE=${OUT} KRYTON_IMAGE_ID=${IMAGE_ID} ${SCRIPT_DIR}/bootstrap-kubevirt-images.sh"
  exit 0
fi

need "${CONTAINER_CLI}"
test -e /dev/kvm || { write_status "failed" "prepare" 0 "/dev/kvm required" "" "" "no kvm"; echo "/dev/kvm required" >&2; exit 1; }
test -d "${OEM_DIR}" || { write_status "failed" "prepare" 0 "OEM dir missing: ${OEM_DIR}" "" "" "oem missing"; exit 1; }

STARTED_AT="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
CONSOLE_PORT="$(pick_port "${CONSOLE_PORT:-8066}")"
CONSOLE_URL="http://${PUBLIC_HOST}:${CONSOLE_PORT}/"

write_status "starting" "pull" 8 "Starting dockur/windows (VERSION=${VERSION})" "${CONSOLE_URL}"

${CONTAINER_CLI} rm -f "${NAME}" >/dev/null 2>&1 || true
PODMAN_EXTRA=()
if [[ "${CONTAINER_CLI}" == "podman" ]]; then
  # Rootless podman often sees /dev/kvm as unwriteable without privileged + keep-groups.
  PODMAN_EXTRA+=(--privileged --group-add=keep-groups --security-opt=label=disable)
fi
${CONTAINER_CLI} run -d --name "${NAME}" \
  -e VERSION="${VERSION}" \
  -e RAM_SIZE="${RAM_SIZE}" \
  -e CPU_CORES="${CPU_CORES}" \
  -e DISK_SIZE="${DISK_SIZE}" \
  -e USERNAME="Kryton" \
  -e PASSWORD="Kryton!" \
  -p "${CONSOLE_PORT}:8006" \
  --device=/dev/kvm \
  --device=/dev/net/tun \
  --cap-add NET_ADMIN \
  "${PODMAN_EXTRA[@]}" \
  -v "${STORAGE}:/storage" \
  -v "${OEM_DIR}:/oem" \
  --stop-timeout 180 \
  "${DOCKUR_IMAGE}" >/dev/null

write_status "installing" "download" 15 "Dockur is downloading Windows media and installing unattended" "${CONSOLE_URL}"

if [[ "${AUTO}" != "1" ]]; then
  cat <<MSG

Golden image builder running (dockur/windows).

  Console:  ${CONSOLE_URL}
  Image ID: ${IMAGE_ID}
  Storage:  ${STORAGE}

Dockur installs Windows fully automatically. Kryton's OEM script will Sysprep on completion.

Watch progress in the browser, or run with --auto to wait and capture automatically.

When done manually:
  BUILD_ID=${BUILD_ID} WORKDIR=${WORKDIR} VERSION=${VERSION} FINALIZE=1 ${SCRIPT_DIR}/build-golden-image.sh
MSG
  exit 0
fi

echo "→ Watching dockur install (auto mode). Console: ${CONSOLE_URL}"
CONSOLE_UP=0
while ${CONTAINER_CLI} inspect -f '{{.State.Running}}' "${NAME}" 2>/dev/null | grep -q true; do
  if curl -fsS --max-time 2 "${CONSOLE_URL}" >/dev/null 2>&1; then
    if [ "${CONSOLE_UP}" -eq 0 ]; then
      CONSOLE_UP=1
      write_status "installing" "windows_setup" 42 "Windows setup running — watch the dockur web viewer" "${CONSOLE_URL}"
    else
      write_status "installing" "windows_setup" 58 "Unattended install in progress (dockur)" "${CONSOLE_URL}"
    fi
  else
    write_status "installing" "download" 28 "Downloading Windows installation media" "${CONSOLE_URL}"
  fi
  sleep 12
done

EXIT_CODE="$(${CONTAINER_CLI} inspect -f '{{.State.ExitCode}}' "${NAME}" 2>/dev/null || echo 1)"
if [ "${EXIT_CODE}" != "0" ]; then
  LOG="$(${CONTAINER_CLI} logs --tail 40 "${NAME}" 2>&1 | tr '"' "'" | tr '\n' ' ')"
  write_status "failed" "install" 0 "Builder container exited unexpectedly" "${CONSOLE_URL}" "" "${LOG}"
  exit 1
fi

write_status "sysprep" "generalize" 82 "Sysprep complete — capturing golden qcow2" "${CONSOLE_URL}"
FINALIZE=1 BUILD_ID="${BUILD_ID}" WORKDIR="${WORKDIR}" VERSION="${VERSION}" \
  IMAGE_ID="${IMAGE_ID}" KRYTON_IMAGE_ID="${IMAGE_ID}" OUT="${OUT}" NAME="${NAME}" STORAGE="${STORAGE}" \
  "${SCRIPT_DIR}/build-golden-image.sh"
