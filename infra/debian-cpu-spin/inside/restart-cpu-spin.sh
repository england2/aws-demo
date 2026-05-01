#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [ ! -f "${SCRIPT_DIR}/.env" ]; then
  echo "missing ${SCRIPT_DIR}/.env"
  echo "create it first or run deploy-cpu-spin.sh <image>"
  exit 1
fi

docker compose -f "${SCRIPT_DIR}/docker-compose.yml" down
docker compose -f "${SCRIPT_DIR}/docker-compose.yml" up -d

echo "cpu-spin restarted using image from ${SCRIPT_DIR}/.env"
