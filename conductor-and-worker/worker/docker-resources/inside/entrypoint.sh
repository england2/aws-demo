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
export GH_TOKEN="$GITHUB_TOKEN"

echo "checking GitHub access for england2/aws-demo"
if ! gh repo view england2/aws-demo >/dev/null; then
	echo "============================================================"
	echo "GitHub authentication setup failed"
	echo "============================================================"
	echo "gh repo view england2/aws-demo failed."
	echo "Check that fine-grained-gh-pat-aws-demo contains a valid GITHUB_TOKEN with access to england2/aws-demo."
	exit 1
fi

# ==============================================================
# Start the worker
# ==============================================================

worker
