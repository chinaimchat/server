#!/usr/bin/env bash
# Download Twemoji 72×72 PNG assets into assets/twemoji72/ so the server can
# serve /v1/common/twemoji72/:name fully offline (no jsDelivr round-trip).
#
# Usage:
#   scripts/fetch-twemoji72.sh                # default: v14.0.2
#   TWEMOJI_VERSION=15.1.0 scripts/fetch-twemoji72.sh
#   FORCE=1 scripts/fetch-twemoji72.sh        # re-download even if dir exists
#
# Network transfer is ~1.5 MB thanks to sparse-checkout + blob filter.
# Safe to re-run: idempotent unless FORCE=1.
set -euo pipefail

VERSION="${TWEMOJI_VERSION:-14.0.2}"
REPO_URL="${TWEMOJI_REPO_URL:-https://github.com/twitter/twemoji.git}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="${ROOT}/assets/twemoji72"

if [[ -d "${OUT}" && -z "${FORCE:-}" ]]; then
  existing=$(find "${OUT}" -maxdepth 1 -name '*.png' 2>/dev/null | wc -l | tr -d ' ')
  if [[ "${existing}" -gt 200 ]]; then
    echo "twemoji72 already present at ${OUT} (${existing} files). Set FORCE=1 to re-download."
    exit 0
  fi
fi

if ! command -v git >/dev/null 2>&1; then
  echo "git is required." >&2
  exit 1
fi

TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

echo "Fetching twemoji@${VERSION} 72x72 assets via sparse checkout..."
git -c advice.detachedHead=false clone \
  --depth=1 --filter=blob:none --sparse \
  --branch="v${VERSION}" \
  "${REPO_URL}" "${TMP}/tw" >/dev/null
(
  cd "${TMP}/tw"
  git sparse-checkout set --no-cone assets/72x72 >/dev/null
)

SRC="${TMP}/tw/assets/72x72"
if [[ ! -d "${SRC}" ]]; then
  echo "sparse-checkout did not produce ${SRC}" >&2
  exit 1
fi

mkdir -p "${OUT}"
# Overwrite-copy; strip the .git checkout noise.
find "${SRC}" -maxdepth 1 -name '*.png' -print0 | xargs -0 -I{} cp "{}" "${OUT}/"

count=$(find "${OUT}" -maxdepth 1 -name '*.png' | wc -l | tr -d ' ')
echo "OK: ${count} files installed to ${OUT}"
