#!/usr/bin/env bash
set -euo pipefail

# ==============================================================
# authenticate with openai
# ==============================================================

if [[ -z "${OPENAI_API_KEY:-}" ]]; then
	OPENAI_API_KEY="$(get-secrets openai-key-aws-demo-agent-fargate | jq -r '.OPENAI_API_KEY')"
	export OPENAI_API_KEY
fi

printenv OPENAI_API_KEY | codex login --with-api-key

# ==============================================================
# authenticate with github
# ==============================================================

GITHUB_TOKEN="$(get-secrets fine-grained-gh-pat-aws-demo | jq -r '.GITHUB_TOKEN')"
export GITHUB_TOKEN

# ==============================================================
# start worker process
# ==============================================================

cd /home/root/work
