#!/bin/bash
set -euo pipefail

if [ "$#" -ne 1 ]; then
  echo "usage: $0 <image>"
  echo "example: $0 204772699175.dkr.ecr.us-west-2.amazonaws.com/cpu-spin:abc123"
  exit 1
fi

IMAGE="$1"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REGISTRY="${IMAGE%%/*}"
REGION="$(echo "$REGISTRY" | cut -d. -f4)"

if ! command -v aws >/dev/null 2>&1; then
  echo "aws CLI is required on the host for ECR login"
  exit 1
fi

aws ecr get-login-password --region "$REGION" | docker login --username AWS --password-stdin "$REGISTRY"

cat > "${SCRIPT_DIR}/.env" <<EOF
CPU_SPIN_IMAGE=${IMAGE}
EOF

docker compose -f "${SCRIPT_DIR}/docker-compose.yml" pull
docker compose -f "${SCRIPT_DIR}/docker-compose.yml" up -d

echo "cpu-spin deployed: ${IMAGE}"
