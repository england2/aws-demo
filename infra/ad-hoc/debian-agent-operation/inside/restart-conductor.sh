#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="${SCRIPT_DIR}/docker-compose.conductor.yml"
ENV_FILE="${SCRIPT_DIR}/.env.conductor"

if [ ! -f "$ENV_FILE" ]; then
  echo "missing $ENV_FILE"
  echo "create it first or run deploy-conductor.sh"
  exit 1
fi

docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" down
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" up -d conductor

echo "conductor restarted without pulling a new image; image came from $ENV_FILE"
