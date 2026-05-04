#!/usr/bin/env bash
set -euo pipefail

# openai codex atuh
if [[ -z "${OPENAI_API_KEY:-}" ]]; then
	OPENAI_API_KEY="$(get-secrets openai-key-aws-demo-agent-fargate | jq -r '.OPENAI_API_KEY')"
	export OPENAI_API_KEY
fi

printenv OPENAI_API_KEY | codex login --with-api-key

# github aws-demo repo auth
export GH_TOKEN="$(get-secrets fine-grained-gh-pat-aws-demo | jq -r '.GITHUB_TOKEN')"
printenv GH_TOKEN | gh auth login --with-token
git config --global credential.helper store
gh auth setup-git

cd /home/root/work
exec /usr/local/bin/codex-wrapper "$@"
