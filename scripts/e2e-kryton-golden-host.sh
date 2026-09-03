#!/usr/bin/env bash
# Sequential Kryton golden builds for all catalog images (host-side, against :9088).
# Usage on lab host: KRYTON_TOKEN=$(cat ~/.kryton/lab.token) ./e2e-kryton-golden-host.sh
set -euo pipefail
URL="${KRYTON_URL:-http://127.0.0.1:9088}"
TOKEN="${KRYTON_TOKEN:?set KRYTON_TOKEN}"
TIMEOUT="${GOLDEN_TIMEOUT_SECS:-9000}"
POLL="${POLL_SECS:-45}"
LEDGER="${LEDGER:-$HOME/kryton-golden-ledger-$(date -u +%Y%m%dT%H%M%SZ).md}"
AUTH=(-H "Authorization: Bearer ${TOKEN}" -H "Content-Type: application/json")

mapfile -t IDS < <(curl -fsS "${AUTH[@]}" "${URL}/api/v1/images" | python3 -c '
import sys,json
d=json.load(sys.stdin)
items=d if isinstance(d,list) else d.get("items") or []
for it in items:
  print(it["id"] if isinstance(it,dict) else it)
')
[[ ${#IDS[@]} -gt 0 ]] || { echo "no images"; exit 1; }

echo "# Kryton host golden ledger" >"$LEDGER"
echo >>"$LEDGER"
echo "| Image | Result | Notes |" >>"$LEDGER"
echo "|-------|--------|-------|" >>"$LEDGER"

for id in "${IDS[@]}"; do
  echo "==== ${id} ===="
  # Skip if already have a ready build for this imageId
  existing=$(curl -fsS "${AUTH[@]}" "${URL}/api/v1/golden" | python3 -c "
import sys,json
d=json.load(sys.stdin)
items=d.get('items') or []
for i in items:
  if i.get('imageId')=='${id}' and i.get('state') in ('ready','Ready'):
    print(i.get('id','')); break
")
  if [[ -n "${existing}" ]]; then
    echo "| \`${id}\` | SKIP-READY | ${existing} |" >>"$LEDGER"
    # still try bootstrap
    curl -fsS -X POST "${AUTH[@]}" "${URL}/api/v1/golden/${existing}/bootstrap" -d '{}' >/dev/null 2>&1 || true
    continue
  fi
  resp=$(curl -fsS -X POST "${AUTH[@]}" "${URL}/api/v1/golden" -d "{\"imageId\":\"${id}\",\"auto\":true}")
  gid=$(printf '%s' "$resp" | python3 -c 'import sys,json; print(json.load(sys.stdin).get("id",""))')
  [[ -n "$gid" ]] || { echo "| \`${id}\` | FAIL | start |" >>"$LEDGER"; continue; }
  echo "golden ${gid}"
  deadline=$((SECONDS+TIMEOUT))
  state=""
  while (( SECONDS < deadline )); do
    g=$(curl -fsS "${AUTH[@]}" "${URL}/api/v1/golden/${gid}" 2>/dev/null || echo '{}')
    state=$(printf '%s' "$g" | python3 -c 'import sys,json; d=json.load(sys.stdin); print(d.get("state") or "")')
    msg=$(printf '%s' "$g" | python3 -c 'import sys,json; d=json.load(sys.stdin); print((d.get("message") or "")[:100])')
    echo "  ${state} ${msg}"
    case "$state" in
      ready|Ready) break ;;
      failed|Failed|error) break ;;
    esac
    sleep "$POLL"
  done
  if [[ "$state" == "ready" || "$state" == "Ready" ]]; then
    curl -fsS -X POST "${AUTH[@]}" "${URL}/api/v1/golden/${gid}/bootstrap" -d '{}' >/dev/null 2>&1 || true
    # smoke create+delete
    name="kz-$(echo "$id" | tr -cd 'a-z0-9-' | cut -c1-36)-$RANDOM"
    mid=$(curl -fsS -X POST "${AUTH[@]}" "${URL}/api/v1/machines" -d "{\"project\":\"default\",\"name\":\"${name}\",\"image\":\"${id}\",\"compute\":{\"cpu\":2,\"memoryMiB\":4096},\"disk\":{\"sizeGiB\":64,\"storageClass\":\"zyvor-rbd-prod\"},\"ttlMinutes\":30}" | python3 -c 'import sys,json; print(json.load(sys.stdin).get("id",""))' 2>/dev/null || true)
    if [[ -n "$mid" ]]; then
      curl -fsS -X DELETE "${AUTH[@]}" "${URL}/api/v1/machines/${mid}?project=default" >/dev/null 2>&1 || true
      echo "| \`${id}\` | OK | golden=${gid} machine=${mid} |" >>"$LEDGER"
    else
      echo "| \`${id}\` | GOLDEN-OK-CREATE-FAIL | golden=${gid} |" >>"$LEDGER"
    fi
  else
    echo "| \`${id}\` | FAIL | state=${state} golden=${gid} |" >>"$LEDGER"
  fi
done
echo "ledger: $LEDGER"
