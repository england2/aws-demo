#!/bin/bash
set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: $0 <git-sha> <artifact-s3-uri>"
  echo "example: $0 abc123 s3://bucket/agent-operation/abc123.tar.gz"
  exit 1
fi

GIT_SHA="$1"
ARTIFACT_URI="$2"
ADMIN_HOME="/home/admin"
BASE_DIR="${ADMIN_HOME}/agent-operation"
CURRENT_DIR="${BASE_DIR}/current"
DATA_DIR="${BASE_DIR}/data"
TMP_DIR="${BASE_DIR}/tmp-${GIT_SHA}"
SERVICE_FILE="/etc/systemd/system/agent-operation.service"

if ! command -v sudo >/dev/null 2>&1; then
  echo "sudo is required"
  exit 1
fi

if ! sudo -u admin test -x "${ADMIN_HOME}/.local/bin/uv"; then
  echo "uv is not installed for admin; run install-agent-operation-host.sh first"
  exit 1
fi

sudo systemctl stop agent-operation.service || true

sudo rm -rf "${TMP_DIR}"
sudo rm -rf "${CURRENT_DIR}"
sudo mkdir -p "${TMP_DIR}"
sudo mkdir -p "${CURRENT_DIR}"
sudo mkdir -p "${DATA_DIR}"
sudo chown -R admin:admin "${BASE_DIR}"

sudo -u admin aws s3 cp "${ARTIFACT_URI}" "${TMP_DIR}/payload.tar.gz"
sudo -u admin tar -xzf "${TMP_DIR}/payload.tar.gz" -C "${TMP_DIR}"

sudo -u admin cp -R "${TMP_DIR}/python/." "${CURRENT_DIR}/"

if [ -f "${TMP_DIR}/agent-operation.service" ]; then
  sudo install -m 0644 "${TMP_DIR}/agent-operation.service" "${SERVICE_FILE}"
fi

sudo chown -R admin:admin "${BASE_DIR}"

PYTHON_VERSION="$(cat "${CURRENT_DIR}/.python-version" 2>/dev/null || echo 3.13)"
sudo -u admin bash -lc "export PATH='${ADMIN_HOME}/.local/bin':\$PATH && cd '${CURRENT_DIR}' && uv python install '${PYTHON_VERSION}' && uv venv --python '${PYTHON_VERSION}' && uv sync --frozen"

sudo systemctl daemon-reload
sudo systemctl start agent-operation.service
sudo systemctl status agent-operation.service --no-pager

sudo rm -rf "${TMP_DIR}"

echo "agent-operation deployed at ${GIT_SHA}"
