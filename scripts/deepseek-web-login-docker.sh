#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
CONFIG_PATH="${DS2API_CONFIG_PATH:-${ROOT_DIR}/config.json}"
ACCOUNT_INDEX="${DS2API_ACCOUNT_INDEX:-0}"
PLAYWRIGHT_IMAGE="${PLAYWRIGHT_IMAGE:-mcr.microsoft.com/playwright/python:v1.55.0-noble}"

if [[ ! -f "${CONFIG_PATH}" ]]; then
  echo "config not found: ${CONFIG_PATH}" >&2
  exit 1
fi

CONFIG_DIR="$(cd "$(dirname "${CONFIG_PATH}")" && pwd)"
CONFIG_FILE="$(basename "${CONFIG_PATH}")"

docker run --rm \
  -v "${ROOT_DIR}:/work:ro" \
  -v "${CONFIG_DIR}:/config" \
  -w /work \
  -e DEEPSEEK_EMAIL="${DEEPSEEK_EMAIL:-}" \
  -e DEEPSEEK_PASSWORD="${DEEPSEEK_PASSWORD:-}" \
  "${PLAYWRIGHT_IMAGE}" \
  python3 /work/scripts/deepseek_web_login.py \
    --config "/config/${CONFIG_FILE}" \
    --account-index "${ACCOUNT_INDEX}" \
    "$@"
