#!/usr/bin/env bash
set -euo pipefail

# ==============================================================
# Populate ~/.codex/auth.json by logging in
# ==============================================================

OPENAI_API_KEY="$(get-secrets openai-key-aws-demo-agent-fargate | jq -r '.OPENAI_API_KEY')"
export OPENAI_API_KEY

# Note: "Successfully logged in" only indicates that the credentials have been stored in Codex's
# local configuration, NOT that the credentials are correct.
echo -e "running: printenv OPENAI_API_KEY | codex login --with-api-key\n"
printenv OPENAI_API_KEY | codex login --with-api-key

# ==============================================================
# Authenticate with github
# ==============================================================

GITHUB_TOKEN="$(get-secrets fine-grained-gh-pat-aws-demo | jq -r '.GITHUB_TOKEN')"
export GITHUB_TOKEN

# ==============================================================
# Start the worker
# ==============================================================

worker
