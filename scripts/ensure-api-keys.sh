#!/usr/bin/env bash
# Create ~/.kryton/keys.json (SHA-256 digests) and lab.token (raw bearer) if missing.
set -euo pipefail

KEYS_DIR="${KRYTON_KEYS_DIR:-${HOME}/.kryton}"
KEYS_FILE="${KRYTON_API_KEYS_FILE:-${KEYS_DIR}/keys.json}"
TOKEN_FILE="${KRYTON_TOKEN_FILE:-${KEYS_DIR}/lab.token}"
KEY_NAME="${KRYTON_KEY_NAME:-lab-admin}"
ROTATE="${KRYTON_ROTATE_KEYS:-false}"

need() { command -v "$1" >/dev/null || { echo "missing: $1" >&2; exit 1; }; }
need krytonctl

mkdir -p "${KEYS_DIR}"
chmod 700 "${KEYS_DIR}"

if [ -f "${KEYS_FILE}" ] && [ -f "${TOKEN_FILE}" ] && [ "${ROTATE}" != "true" ]; then
  echo "API keys OK: ${KEYS_FILE}"
  exit 0
fi

TOKEN="$(krytonctl generate-token)"
HASH="$(krytonctl hash-token "${TOKEN}")"

cat >"${KEYS_FILE}" <<EOF
{
  "keys": [
    {
      "name": "${KEY_NAME}",
      "sha256": "${HASH}",
      "role": "admin",
      "projects": ["*"]
    }
  ]
}
EOF
printf '%s\n' "${TOKEN}" >"${TOKEN_FILE}"
chmod 600 "${KEYS_FILE}" "${TOKEN_FILE}"

echo "Created ${KEYS_FILE}"
echo "Bearer token written to ${TOKEN_FILE}"
echo ""
echo "Use in browser: Settings → paste token, or session auth on first 401."
echo "CLI: export KRYTON_TOKEN=\$(cat ${TOKEN_FILE})"
