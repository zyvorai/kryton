#!/usr/bin/env bash
# Copyright 2026 Kryton contributors
# SPDX-License-Identifier: Apache-2.0
# Add Apache-2.0 headers to Kryton first-party source files (idempotent).
# --check: report files missing a header without modifying anything; exits
# non-zero if any are found. Used by CI to enforce headers on new files.
set -euo pipefail

CHECK=0
if [ "${1:-}" = "--check" ]; then
  CHECK=1
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"
MISSING=()

GO_HEADER='// Copyright 2026 Kryton contributors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
'

JS_HEADER='// Copyright 2026 Kryton contributors
// SPDX-License-Identifier: Apache-2.0
'

CSS_HEADER='/* Copyright 2026 Kryton contributors
 * SPDX-License-Identifier: Apache-2.0
 */
'

YAML_HEADER='# Copyright 2026 Kryton contributors
# SPDX-License-Identifier: Apache-2.0
'

has_apache() {
  grep -q 'Apache License, Version 2.0\|SPDX-License-Identifier: Apache-2.0' "$1" 2>/dev/null
}

prepend_go() {
  local f="$1"
  has_apache "$f" && return 0
  if [ "${CHECK}" -eq 1 ]; then
    MISSING+=("$f")
    return 0
  fi
  local tmp
  tmp="$(mktemp)"
  printf '%s\n' "${GO_HEADER}" >"${tmp}"
  cat "$f" >>"${tmp}"
  mv "${tmp}" "$f"
  echo "  go: $f"
}

prepend_text() {
  local f="$1" header="$2"
  has_apache "$f" && return 0
  if [ "${CHECK}" -eq 1 ]; then
    MISSING+=("$f")
    return 0
  fi
  local tmp
  tmp="$(mktemp)"
  printf '%s\n' "${header}" >"${tmp}"
  cat "$f" >>"${tmp}"
  mv "${tmp}" "$f"
  echo "  + $f"
}

prepend_shell() {
  local f="$1"
  has_apache "$f" && return 0
  if [ "${CHECK}" -eq 1 ]; then
    MISSING+=("$f")
    return 0
  fi
  local tmp first
  tmp="$(mktemp)"
  first="$(head -n1 "$f")"
  {
    echo "${first}"
    echo "# Copyright 2026 Kryton contributors"
    echo "# SPDX-License-Identifier: Apache-2.0"
    tail -n +2 "$f"
  } >"${tmp}"
  mv "${tmp}" "$f"
  echo "  sh: $f"
}

echo "→ Go files"
while IFS= read -r f; do
  prepend_go "$f"
done < <(find . -name '*.go' -not -path './node_modules/*' | sort)

echo "→ Shell scripts"
for f in scripts/*.sh; do
  [ -f "$f" ] || continue
  prepend_shell "$f"
done

echo "→ Web assets"
for f in cmd/krytond/web/app.js cmd/krytond/web/console-viewer.js; do
  [ -f "$f" ] && prepend_text "$f" "${JS_HEADER}"
done
[ -f cmd/krytond/web/style.css ] && prepend_text cmd/krytond/web/style.css "${CSS_HEADER}"

echo "→ OpenAPI YAML"
for f in openapi.yaml cmd/krytond/openapi.yaml internal/api/openapi.yaml; do
  [ -f "$f" ] && prepend_text "$f" "${YAML_HEADER}"
done

echo "→ Makefile"
if [ -f Makefile ] && ! has_apache Makefile; then
  if [ "${CHECK}" -eq 1 ]; then
    MISSING+=("Makefile")
  else
    tmp="$(mktemp)"
    {
      echo "# Copyright 2026 Kryton contributors"
      echo "# SPDX-License-Identifier: Apache-2.0"
      cat Makefile
    } >"${tmp}"
    mv "${tmp}" Makefile
    echo "  makefile: Makefile"
  fi
fi

if [ "${CHECK}" -eq 1 ]; then
  if [ "${#MISSING[@]}" -gt 0 ]; then
    echo "✗ Missing Apache-2.0 header (${#MISSING[@]}):"
    printf '  %s\n' "${MISSING[@]}"
    echo "Run ./scripts/add-license-headers.sh to fix."
    exit 1
  fi
  echo "✓ All files have a license header"
  exit 0
fi

echo "✓ Done"
