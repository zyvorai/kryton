#!/usr/bin/env bash
# Render Kryton social cards with Google Chrome + macOS `sips`.
#   ./docs/social/build-social-card.sh
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
CHROME="${CHROME:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}"
[[ -x "$CHROME" ]] || { echo "Google Chrome not found (set CHROME=...)" >&2; exit 1; }

SOCIAL_OUT="${1:-$HERE/kryton-social-card.jpg}"
SHARE_OUT="${2:-$HERE/kryton-share-card.png}"

PNG="$(mktemp "${TMPDIR:-/tmp}/kryton-social.XXXXXX.png")"
PNG2="$(mktemp "${TMPDIR:-/tmp}/kryton-share.XXXXXX.png")"
trap 'rm -f "$PNG" "$PNG2"' EXIT

"$CHROME" --headless=new --disable-gpu --hide-scrollbars --force-device-scale-factor=1 \
  --window-size=1600,900 --screenshot="$PNG" "file://$HERE/kryton-social-card.html" >/dev/null 2>&1
sips -s format jpeg -s formatOptions 92 "$PNG" --out "$SOCIAL_OUT" >/dev/null
echo "wrote $SOCIAL_OUT ($(sips -g pixelWidth -g pixelHeight "$SOCIAL_OUT" | awk '/pixel/{printf "%s ", $2}')px, $(du -k "$SOCIAL_OUT" | cut -f1) KB)"

"$CHROME" --headless=new --disable-gpu --hide-scrollbars --force-device-scale-factor=1 \
  --window-size=1200,630 --screenshot="$PNG2" "file://$HERE/kryton-share-card.html" >/dev/null 2>&1
sips -s format png "$PNG2" --out "$SHARE_OUT" >/dev/null
echo "wrote $SHARE_OUT ($(sips -g pixelWidth -g pixelHeight "$SHARE_OUT" | awk '/pixel/{printf "%s ", $2}')px, $(du -k "$SHARE_OUT" | cut -f1) KB)"
