#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ADMIN_HOME="/home/admin"
BASE_DIR="${ADMIN_HOME}/agent-operation"

sudo apt update
sudo apt install -y git curl ca-certificates awscli tmux

sudo mkdir -p "${BASE_DIR}/current"
sudo mkdir -p "${BASE_DIR}/data"
sudo chown -R admin:admin "${BASE_DIR}"

if ! sudo -u admin test -x "${ADMIN_HOME}/.local/bin/uv"; then
  sudo -u admin bash -lc 'curl -LsSf https://astral.sh/uv/install.sh | sh'
fi

sudo install -m 0644 "${SCRIPT_DIR}/agent-operation.service" /etc/systemd/system/agent-operation.service
sudo systemctl daemon-reload
sudo systemctl enable agent-operation.service

echo "agent-operation host bootstrap complete"
echo "uv installed at ${ADMIN_HOME}/.local/bin/uv"
echo "service installed at /etc/systemd/system/agent-operation.service"
