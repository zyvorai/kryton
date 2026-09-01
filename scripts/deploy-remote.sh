#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# ─────────────────────────────────────────────────────────────
# Kryton — Remote deployment (SSH + rsync)
#
# Profiles:
#   default     Sync → ensure Go → build on remote → install → start demo service
#   --quick     Rsync + remote build only (skip system Go install if present)
#   --quick --build-local   Rsync pre-built Linux binaries (build locally first)
#
# Auth: SSH keys (recommended). Password via sshpass is supported but deprecated.
# ─────────────────────────────────────────────────────────────
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
VERSION="1.0.0"
REMOTE_DIR=""
DEPLOY_PROFILE="full"
DEPLOY_LOG="${KRYTON_DEPLOY_LOG:-${HOME}/.kryton/deploy-$(date +%Y%m%d-%H%M%S).log}"
KRYTON_PORT="${KRYTON_PORT:-8080}"

QUICK_MODE=false
UNINSTALL=false
KEY_AUTH=false
DRY_RUN=false
SKIP_SYNC=false
SKIP_VERIFY=false
BUILD_LOCAL=false
VERIFY_ONLY=false
PREFLIGHT_ONLY=false
NO_SERVICE=false
VERBOSE=false
SSH_RETRIES="${KRYTON_SSH_RETRIES:-3}"
POSITIONAL=()

usage() {
    cat <<EOF
Kryton remote deploy v${VERSION}

Usage:
  $0 <host> <user> [options]
  $0 user@host [options]

Profiles:
  (default)                 Full remote build + Go toolchain if needed + systemd demo unit
  --quick                   Rsync + go build on remote (skip toolchain install when Go exists)
  --quick --build-local     Install locally built Linux binaries (Linux build host required)

Options:
  --help              Show this help
  --dry-run           Print steps without SSH/rsync/build
  --preflight-only    SSH + disk/sudo checks, then exit
  --verify-only       Hit /readyz on the remote host only
  --skip-sync         Skip rsync (sources already on host)
  --skip-verify       Skip health check
  --build-local       With --quick: use local bin/krytond + bin/krytonctl
  --no-service        Install binaries only (do not enable systemd unit)
  --key               SSH key auth (clear password)
  --uninstall         Stop service and remove install
  -v, --verbose       Verbose rsync

Environment:
  KRYTON_DEPLOY_LOG    Log file path
  KRYTON_SSH_RETRIES   SSH retry count (default: 3)
  KRYTON_PORT          Listen port for demo unit (default: 8080)
  DEPLOY_DIR           Override remote staging dir (default: ~/.deployments/kryton)

Examples:
  $0 10.0.0.5 root --key
  $0 sus@175.110.122.71 --quick
  $0 10.0.0.5 root --build-local --quick
  make deploy-remote H=10.0.0.5 U=root
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        -h|--help)        usage; exit 0 ;;
        --quick)          QUICK_MODE=true; DEPLOY_PROFILE="quick"; shift ;;
        --uninstall)      UNINSTALL=true; shift ;;
        --key)            KEY_AUTH=true; shift ;;
        --dry-run)        DRY_RUN=true; shift ;;
        --skip-sync)      SKIP_SYNC=true; shift ;;
        --skip-verify)    SKIP_VERIFY=true; shift ;;
        --build-local)    BUILD_LOCAL=true; shift ;;
        --verify-only)    VERIFY_ONLY=true; shift ;;
        --preflight-only) PREFLIGHT_ONLY=true; shift ;;
        --no-service)     NO_SERVICE=true; shift ;;
        -v|--verbose)     VERBOSE=true; shift ;;
        *)
            POSITIONAL+=("$1")
            shift
            ;;
    esac
done

TARGET_HOST="${POSITIONAL[0]:-}"
TARGET_USER="${POSITIONAL[1]:-root}"
TARGET_PASS="${POSITIONAL[2]:-}"

if [ "$KEY_AUTH" = true ]; then
    TARGET_PASS=""
fi

if [[ -n "${TARGET_HOST}" && "${TARGET_HOST}" == *"@"* ]]; then
    TARGET_USER="${TARGET_HOST%%@*}"
    TARGET_HOST="${TARGET_HOST#*@}"
fi

_use_color() { [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; }
if _use_color; then
    C_OK=$'\033[32m'; C_FAIL=$'\033[31m'; C_INFO=$'\033[36m'; C_WARN=$'\033[33m'
    C_DIM=$'\033[2m'; C_BOLD=$'\033[1m'; C_CYAN=$'\033[96m'; C_RST=$'\033[0m'
else
    C_OK= C_FAIL= C_INFO= C_WARN= C_DIM= C_BOLD= C_CYAN= C_RST=
fi

_log_file() { mkdir -p "$(dirname "$DEPLOY_LOG")" 2>/dev/null || true; echo "[$(date -Iseconds)] $*" >>"$DEPLOY_LOG" 2>/dev/null || true; }
ok()   { echo "${C_OK}  ✓ $*${C_RST}"; _log_file "OK $*"; }
fail() { echo "${C_FAIL}  ✗ $*${C_RST}" >&2; _log_file "FAIL $*"; exit 1; }
info() { echo "${C_INFO}  → $*${C_RST}"; _log_file "INFO $*"; }
warn() { echo "${C_WARN}  ! $*${C_RST}"; _log_file "WARN $*"; }
dry()  { echo "${C_DIM}  (dry) $*${C_RST}"; _log_file "DRY $*"; }

print_banner() {
    local target="${TARGET_USER}@${TARGET_HOST}"
    echo ""
    echo "${C_CYAN}${C_BOLD}  Kryton remote deploy${C_RST}  ${C_DIM}v${VERSION}${C_RST}"
    echo "${C_DIM}  ${target}  ·  ${DEPLOY_PROFILE}${C_RST}"
    [ "$DRY_RUN" = true ] && echo "${C_WARN}  dry-run — no remote changes${C_RST}"
    echo ""
}

STEP_T0=0
STEP_IDX=0
step_begin() {
    STEP_IDX=$((STEP_IDX + 1))
    STEP_T0=$(date +%s)
    echo ""
    echo "${C_BOLD}${C_CYAN}  Step ${STEP_IDX}: $*${C_RST}"
    _log_file "STEP ${STEP_IDX}: $*"
}
step_end() { echo "${C_DIM}  finished in $(( $(date +%s) - STEP_T0 ))s${C_RST}"; }

SSH_OPTS="-o StrictHostKeyChecking=accept-new -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=15 -o ServerAliveInterval=30"
if [ -z "${TARGET_PASS}" ]; then
    SSH_OPTS+=" -o BatchMode=yes -o PreferredAuthentications=publickey"
fi

_ssh_once() {
    if [ -n "${TARGET_PASS}" ] && command -v sshpass &>/dev/null; then
        export SSHPASS="${TARGET_PASS}"
        sshpass -e ssh ${SSH_OPTS} "${TARGET_USER}@${TARGET_HOST}" "$@"
    else
        ssh ${SSH_OPTS} "${TARGET_USER}@${TARGET_HOST}" "$@"
    fi
}

_ssh() {
    local attempt=1 max="${SSH_RETRIES}"
    while [ "$attempt" -le "$max" ]; do
        if _ssh_once "$@"; then
            return 0
        fi
        attempt=$((attempt + 1))
        if [ "$attempt" -le "$max" ]; then
            local _d=$(( 2 * (attempt - 1) )); _d=$(( _d < 2 ? 2 : _d > 30 ? 30 : _d ))
            warn "SSH retry ${attempt}/${max}" && sleep "${_d}"
        fi
    done
    return 1
}

_rsync() {
    local opts="-az --delete"
    [ "$VERBOSE" = true ] && opts+=" --progress"
    if [ -n "${TARGET_PASS}" ] && command -v sshpass &>/dev/null; then
        export SSHPASS="${TARGET_PASS}"
        rsync ${opts} -e "sshpass -e ssh ${SSH_OPTS}" "$@"
    else
        rsync ${opts} -e "ssh ${SSH_OPTS}" "$@"
    fi
}

validate() {
    [ -n "${TARGET_HOST}" ] || { usage; exit 1; }
    [ -f "${PROJECT_DIR}/go.mod" ] || fail "Not in kryton repo: ${PROJECT_DIR}"
    if [ -n "${TARGET_PASS}" ]; then
        warn "Password auth is deprecated. Prefer: ssh-copy-id ${TARGET_USER}@${TARGET_HOST}"
        command -v sshpass &>/dev/null || fail "sshpass required for password auth"
    fi
}

check_connectivity() {
    info "SSH → ${TARGET_USER}@${TARGET_HOST}  log: ${DEPLOY_LOG}"
    if [ "$DRY_RUN" = true ]; then
        REMOTE_DIR="${DEPLOY_DIR:-${HOME}/.deployments/kryton}"
        return 0
    fi
    _ssh "echo ok" &>/dev/null || fail "SSH failed — try: ssh-copy-id ${TARGET_USER}@${TARGET_HOST}"
    ok "SSH connected"
    local remote_home
    remote_home=$(_ssh "echo \$HOME" 2>/dev/null | tr -d '\r')
    remote_home="${remote_home:-/home/${TARGET_USER}}"
    REMOTE_DIR="${DEPLOY_DIR:-${remote_home}/.deployments/kryton}"
    info "Remote path: ${REMOTE_DIR}"
}

preflight_remote() {
    info "Preflight on ${TARGET_HOST}..."
    if [ "$DRY_RUN" = true ]; then return 0; fi
    _ssh bash <<'REMOTE' || fail "Preflight failed"
set -e
echo "  host: $(hostname -f 2>/dev/null || hostname)"
echo "  os:   $(. /etc/os-release 2>/dev/null && echo "$PRETTY_NAME" || uname -s)"
echo "  arch: $(uname -m)"
echo "  disk: $(df -h / 2>/dev/null | awk 'NR==2{print $4 " free"}' || echo n/a)"
if [ "$(id -u)" -ne 0 ]; then
    if ! sudo -n true 2>/dev/null; then
        echo "  non-root user needs passwordless sudo for install"
        exit 1
    fi
    echo "  passwordless sudo: ok"
else
    echo "  running as root"
fi
command -v curl >/dev/null && echo "  curl: ok" || echo "  curl: missing (needed to fetch Go)"
REMOTE
    ok "Preflight passed"
}

build_local_artifacts() {
    step_begin "Local build (Linux release)"
    if [ "$DRY_RUN" = true ]; then
        dry "would run: make build"
        step_end
        return 0
    fi
    if [ "$(uname -s)" != "Linux" ]; then
        fail "--build-local requires a Linux build host (same arch as remote)"
    fi
    (cd "${PROJECT_DIR}" && make build)
    [ -f "${PROJECT_DIR}/bin/krytond" ] || fail "bin/krytond missing"
    [ -f "${PROJECT_DIR}/bin/krytonctl" ] || fail "bin/krytonctl missing"
    ok "Local binaries ready"
    step_end
}

sync_files() {
    if [ "$SKIP_SYNC" = true ]; then
        info "Skipping rsync (--skip-sync)"
        return 0
    fi
    if [ "$DRY_RUN" = true ]; then
        dry "would rsync → ${REMOTE_DIR}"
        return 0
    fi
    step_begin "Sync source"
    _ssh "mkdir -p '${REMOTE_DIR}'"
    _rsync \
        --exclude '.git' \
        --exclude 'bin' \
        --exclude '.ux-shots' \
        --exclude '.deploy-last' \
        --exclude '*.png' \
        "${PROJECT_DIR}/" "${TARGET_USER}@${TARGET_HOST}:${REMOTE_DIR}/"
    ok "Source synced to ${REMOTE_DIR}"
    step_end
}

sync_binaries_only() {
    step_begin "Sync release binaries"
    if [ "$DRY_RUN" = true ]; then
        dry "would rsync bin/krytond bin/krytonctl"
        step_end
        return 0
    fi
    [ -f "${PROJECT_DIR}/bin/krytond" ] || fail "Missing bin/krytond"
    _ssh "mkdir -p '${REMOTE_DIR}/bin'"
    _rsync "${PROJECT_DIR}/bin/krytond" "${TARGET_USER}@${TARGET_HOST}:${REMOTE_DIR}/bin/krytond"
    _rsync "${PROJECT_DIR}/bin/krytonctl" "${TARGET_USER}@${TARGET_HOST}:${REMOTE_DIR}/bin/krytonctl"
    ok "Binaries synced"
    step_end
}

ensure_go_remote() {
    step_begin "Ensure Go toolchain"
    if [ "$DRY_RUN" = true ]; then
        dry "would ensure go 1.23+"
        step_end
        return 0
    fi
    if [ "$QUICK_MODE" = true ]; then
        if _ssh "command -v go >/dev/null"; then
            ok "Go already present (quick)"
            step_end
            return 0
        fi
        warn "Go missing on remote — installing anyway"
    fi
    _ssh bash <<'REMOTE'
set -euo pipefail
if command -v go >/dev/null 2>&1; then
    ver=$(go env GOVERSION 2>/dev/null || go version)
    echo "Go present: ${ver}"
    exit 0
fi
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) GOARCH=amd64 ;;
  aarch64|arm64) GOARCH=arm64 ;;
  *) echo "unsupported arch: $ARCH"; exit 1 ;;
esac
GO_VER="${KRYTON_GO_VERSION:-1.23.8}"
TMP=$(mktemp -d)
curl -fsSL "https://go.dev/dl/go${GO_VER}.linux-${GOARCH}.tar.gz" -o "${TMP}/go.tgz"
SUDO=""; [ "$(id -u)" -ne 0 ] && SUDO="sudo"
$SUDO rm -rf /usr/local/go
$SUDO tar -C /usr/local -xzf "${TMP}/go.tgz"
rm -rf "$TMP"
echo 'export PATH=/usr/local/go/bin:$PATH' | $SUDO tee /etc/profile.d/go.sh >/dev/null
export PATH=/usr/local/go/bin:$PATH
go version
REMOTE
    ok "Go ready"
    step_end
}

build_install_remote() {
    step_begin "Build + install on remote"
    if [ "$DRY_RUN" = true ]; then
        dry "would go build and install to /usr/local/bin"
        step_end
        return 0
    fi
    _ssh env REMOTE_STAGING="${REMOTE_DIR}" KRYTON_PORT="${KRYTON_PORT}" NO_SERVICE="$NO_SERVICE" bash <<'REMOTE'
set -euo pipefail
SUDO=""; [ "$(id -u)" -ne 0 ] && SUDO="sudo"
export PATH=/usr/local/go/bin:${HOME}/go/bin:${PATH}
cd "${REMOTE_STAGING}"
mkdir -p bin
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o bin/krytond ./cmd/krytond
CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o bin/krytonctl ./cmd/krytonctl
$SUDO install -m755 bin/krytond /usr/local/bin/krytond
$SUDO install -m755 bin/krytonctl /usr/local/bin/krytonctl
krytond -h >/dev/null 2>&1 || true
echo "Installed: $(command -v krytond) $(command -v krytonctl)"

if [ "${NO_SERVICE}" = "true" ]; then
  echo "Skipping systemd unit (--no-service)"
  exit 0
fi

$SUDO tee /etc/systemd/system/kryton.service >/dev/null <<UNIT
[Unit]
Description=Kryton Windows virtualization control plane
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
Environment=KRYTON_PROVIDER=demo
Environment=KRYTON_AUTH_MODE=disabled
Environment=KRYTON_ADDR=:${KRYTON_PORT}
Environment=KRYTON_ALLOW_INSECURE=true
ExecStart=/usr/local/bin/krytond
Restart=on-failure
RestartSec=3
User=root

[Install]
WantedBy=multi-user.target
UNIT
$SUDO systemctl daemon-reload
$SUDO systemctl enable --now kryton.service
$SUDO systemctl --no-pager --full status kryton.service | head -20 || true
REMOTE
    ok "Binaries installed"
    step_end
}

install_binaries_quick() {
    step_begin "Install synced binaries"
    if [ "$DRY_RUN" = true ]; then
        dry "would install remote bin/* to /usr/local/bin"
        step_end
        return 0
    fi
    _ssh env REMOTE_STAGING="${REMOTE_DIR}" KRYTON_PORT="${KRYTON_PORT}" NO_SERVICE="$NO_SERVICE" bash <<'REMOTE'
set -euo pipefail
SUDO=""; [ "$(id -u)" -ne 0 ] && SUDO="sudo"
$SUDO install -m755 "${REMOTE_STAGING}/bin/krytond" /usr/local/bin/krytond
$SUDO install -m755 "${REMOTE_STAGING}/bin/krytonctl" /usr/local/bin/krytonctl
if [ "${NO_SERVICE}" = "true" ]; then
  echo "Skipping systemd unit (--no-service)"
  exit 0
fi
$SUDO tee /etc/systemd/system/kryton.service >/dev/null <<UNIT
[Unit]
Description=Kryton Windows virtualization control plane
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
Environment=KRYTON_PROVIDER=demo
Environment=KRYTON_AUTH_MODE=disabled
Environment=KRYTON_ADDR=:${KRYTON_PORT}
Environment=KRYTON_ALLOW_INSECURE=true
ExecStart=/usr/local/bin/krytond
Restart=on-failure
RestartSec=3
User=root

[Install]
WantedBy=multi-user.target
UNIT
$SUDO systemctl daemon-reload
$SUDO systemctl enable --now kryton.service
echo "Installed binaries + service"
REMOTE
    ok "Quick install done"
    step_end
}

verify_remote() {
    step_begin "Verify health"
    if [ "$DRY_RUN" = true ]; then
        dry "would curl http://${TARGET_HOST}:${KRYTON_PORT}/readyz"
        step_end
        return 0
    fi
    local url="http://${TARGET_HOST}:${KRYTON_PORT}/readyz"
    local i=0
    while [ "$i" -lt 30 ]; do
        if curl -fsS --connect-timeout 2 "$url" >/dev/null 2>&1; then
            ok "Healthy at ${url}"
            info "UI: http://${TARGET_HOST}:${KRYTON_PORT}/"
            step_end
            return 0
        fi
        # Fall back to SSH-local curl if host port not reachable from here
        if _ssh "curl -fsS --connect-timeout 2 http://127.0.0.1:${KRYTON_PORT}/readyz" >/dev/null 2>&1; then
            ok "Healthy on remote localhost:${KRYTON_PORT} (open firewall / tunnel for public access)"
            info "ssh -L ${KRYTON_PORT}:127.0.0.1:${KRYTON_PORT} ${TARGET_USER}@${TARGET_HOST}"
            step_end
            return 0
        fi
        i=$((i + 1))
        sleep 1
    done
    fail "Health check failed for port ${KRYTON_PORT}"
}

uninstall_remote() {
    step_begin "Uninstall"
    if [ "$DRY_RUN" = true ]; then
        dry "would stop kryton.service and remove binaries/staging"
        step_end
        return 0
    fi
    _ssh env REMOTE_STAGING="${REMOTE_DIR}" bash <<'REMOTE'
set -euo pipefail
SUDO=""; [ "$(id -u)" -ne 0 ] && SUDO="sudo"
$SUDO systemctl disable --now kryton.service 2>/dev/null || true
$SUDO rm -f /etc/systemd/system/kryton.service
$SUDO systemctl daemon-reload 2>/dev/null || true
$SUDO rm -f /usr/local/bin/krytond /usr/local/bin/krytonctl
rm -rf "${REMOTE_STAGING}"
echo "Removed Kryton install"
REMOTE
    ok "Uninstalled"
    step_end
}

main() {
    print_banner
    validate
    check_connectivity

    if [ "$UNINSTALL" = true ]; then
        uninstall_remote
        exit 0
    fi
    if [ "$PREFLIGHT_ONLY" = true ]; then
        preflight_remote
        exit 0
    fi
    if [ "$VERIFY_ONLY" = true ]; then
        verify_remote
        exit 0
    fi

    run_step_preflight() { step_begin "Preflight"; preflight_remote; step_end; }
    run_step_preflight

    if [ "$BUILD_LOCAL" = true ]; then
        build_local_artifacts
        sync_binaries_only
        install_binaries_quick
    else
        sync_files
        if [ "$QUICK_MODE" = false ] || ! _ssh "command -v go >/dev/null" 2>/dev/null; then
            ensure_go_remote
        else
            info "Skipping Go install (quick + go present)"
        fi
        build_install_remote
    fi

    if [ "$SKIP_VERIFY" = false ] && [ "$NO_SERVICE" = false ]; then
        verify_remote
    fi

    echo ""
    ok "Deploy complete"
    info "Docs: docs/DEPLOY-REMOTE.md"
}

main
