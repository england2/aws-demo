#!/bin/bash
set -euo pipefail

DEFAULT_CONDUCTOR_IMAGE="204772699175.dkr.ecr.us-west-2.amazonaws.com/conductor:latest"

if [ "$#" -gt 1 ]; then
  echo "usage: $0 [image]"
  echo "example: $0 204772699175.dkr.ecr.us-west-2.amazonaws.com/conductor:latest"
  exit 1
fi

CONDUCTOR_IMAGE="${1:-$DEFAULT_CONDUCTOR_IMAGE}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REGISTRY="${CONDUCTOR_IMAGE%%/*}"
REGION="$(echo "$REGISTRY" | cut -d. -f4)"
COMPOSE_FILE="${SCRIPT_DIR}/docker-compose.conductor.yml"
ENV_FILE="${SCRIPT_DIR}/.env.conductor"

if ! command -v aws >/dev/null 2>&1; then
  echo "aws CLI is required on the host for ECR login"
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required on the host; run install-docker.sh first"
  exit 1
fi

if [ ! -f "$COMPOSE_FILE" ]; then
  echo "missing $COMPOSE_FILE"
  exit 1
fi

sudo mkdir -p /conductor/data /conductor/run
sudo chown -R admin:admin /conductor

aws ecr get-login-password --region "$REGION" | docker login --username AWS --password-stdin "$REGISTRY"

cat > "$ENV_FILE" <<EOF
CONDUCTOR_IMAGE=${CONDUCTOR_IMAGE}
EOF

docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" pull conductor
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" up -d --force-recreate conductor

sleep 3
if [ "$(docker inspect -f '{{.State.Running}}' conductor)" != "true" ]; then
  docker logs conductor || true
  echo "conductor container exited after deploy"
  exit 1
fi

echo "conductor deployed: ${CONDUCTOR_IMAGE}"
echo "inspect with:"
echo "  docker logs -f conductor"
echo "  docker exec -it conductor /bin/bash"
