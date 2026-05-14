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
CONDUCTOR_RUN_DIR="/conductor/run"
CONDUCTOR_SHUTDOWN_REQUEST_FILE="${CONDUCTOR_RUN_DIR}/IS_CONDUCTOR_SHUTTING_DOWN"
CONDUCTOR_READY_FOR_SAFE_SHUTDOWN_FILE="${CONDUCTOR_RUN_DIR}/CONDUCTOR_READY_FOR_SAFE_SHUTDOWN"
SAFE_SHUTDOWN_TIMEOUT_SECONDS="${SAFE_SHUTDOWN_TIMEOUT_SECONDS:-900}"

if ! command -v aws >/dev/null 2>&1; then
  echo "aws CLI is required on the host for ECR login"
  exit 1
fi

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required on the host; run install-docker.sh first"
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required on the host to read the EC2 private IP"
  exit 1
fi

if [ ! -f "$COMPOSE_FILE" ]; then
  echo "missing $COMPOSE_FILE"
  exit 1
fi

is_conductor_container_running() {
  [ "$(docker inspect -f '{{.State.Running}}' conductor 2>/dev/null || true)" = "true" ]
}

request_existing_conductor_safe_shutdown() {
  if ! docker inspect conductor >/dev/null 2>&1; then
    echo "no existing conductor container to stop"
    return 0
  fi

  if ! is_conductor_container_running; then
    echo "existing conductor container is already stopped"
    return 0
  fi

  echo "requesting safe shutdown from existing conductor"
  mkdir -p "$CONDUCTOR_RUN_DIR"
  rm -f "$CONDUCTOR_READY_FOR_SAFE_SHUTDOWN_FILE"
  printf 'true\n' > "$CONDUCTOR_SHUTDOWN_REQUEST_FILE"

  deadline=$((SECONDS + SAFE_SHUTDOWN_TIMEOUT_SECONDS))
  while true; do
    ready_for_safe_shutdown="false"
    if [ -f "$CONDUCTOR_READY_FOR_SAFE_SHUTDOWN_FILE" ]; then
      ready_for_safe_shutdown="$(tr -d '[:space:]' < "$CONDUCTOR_READY_FOR_SAFE_SHUTDOWN_FILE")"
    fi

    if [ "$ready_for_safe_shutdown" = "true" ] && ! is_conductor_container_running; then
      echo "existing conductor reached safe shutdown and exited"
      return 0
    fi

    if ! is_conductor_container_running && [ "$ready_for_safe_shutdown" != "true" ]; then
      docker logs conductor || true
      echo "existing conductor exited before writing safe shutdown signal"
      exit 1
    fi

    if [ "$SECONDS" -ge "$deadline" ]; then
      docker logs conductor || true
      echo "timed out waiting for conductor safe shutdown after ${SAFE_SHUTDOWN_TIMEOUT_SECONDS}s"
      exit 1
    fi

    echo "waiting for conductor safe shutdown signal..."
    sleep 2
  done
}

sudo mkdir -p /conductor/data /conductor/run
sudo chown -R admin:admin /conductor

aws ecr get-login-password --region "$REGION" | docker login --username AWS --password-stdin "$REGISTRY"

IMDS_TOKEN="$(curl -fsS -X PUT "http://169.254.169.254/latest/api/token" -H "X-aws-ec2-metadata-token-ttl-seconds: 21600")"
INSTANCE_PRIVATE_IP="$(curl -fsS -H "X-aws-ec2-metadata-token: ${IMDS_TOKEN}" "http://169.254.169.254/latest/meta-data/local-ipv4")"
CONDUCTOR_WORKER_DIAL_ADDR="${CONDUCTOR_WORKER_DIAL_ADDR:-${INSTANCE_PRIVATE_IP}:50055}"

cat > "$ENV_FILE" <<EOF
CONDUCTOR_IMAGE=${CONDUCTOR_IMAGE}
CONDUCTOR_WORKER_DIAL_ADDR=${CONDUCTOR_WORKER_DIAL_ADDR}
EOF

docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" pull conductor
request_existing_conductor_safe_shutdown
rm -f "$CONDUCTOR_READY_FOR_SAFE_SHUTDOWN_FILE"
docker compose --env-file "$ENV_FILE" -f "$COMPOSE_FILE" up -d --force-recreate conductor

sleep 3
if [ "$(docker inspect -f '{{.State.Running}}' conductor)" != "true" ]; then
  docker logs conductor || true
  echo "conductor container exited after deploy"
  exit 1
fi

echo "conductor deployed: ${CONDUCTOR_IMAGE}"
echo "worker dial address: ${CONDUCTOR_WORKER_DIAL_ADDR}"
echo "inspect with:"
echo "  docker logs -f conductor"
echo "  docker exec -it conductor /bin/bash"
