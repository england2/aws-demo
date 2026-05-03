#!/bin/bash
set -euo pipefail

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

cd "$TMP_DIR"
curl -fsSLO https://s3.us-west-2.amazonaws.com/amazon-ssm-us-west-2/latest/debian_amd64/amazon-ssm-agent.deb
sudo dpkg -i amazon-ssm-agent.deb
sudo systemctl enable amazon-ssm-agent
sudo systemctl start amazon-ssm-agent

echo "SSM Agent installed."
echo "Verify with:"
echo "  systemctl status amazon-ssm-agent --no-pager"
