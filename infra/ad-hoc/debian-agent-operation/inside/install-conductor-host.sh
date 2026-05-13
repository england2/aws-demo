#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

sudo apt update
sudo apt install -y ca-certificates awscli

if ! command -v docker >/dev/null 2>&1; then
  bash "${SCRIPT_DIR}/install-docker.sh"
fi

sudo mkdir -p /conductor/data /conductor/run
sudo chown -R admin:admin /conductor

echo "conductor host bootstrap complete"
echo "runtime state is stored in /conductor and bind-mounted into the conductor container"
echo "this setup intentionally does not install a systemd unit or Docker restart policy"
