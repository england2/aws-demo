#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${OPENAI_API_KEY:-}" ]]; then
	OPENAI_API_KEY="$(get-secrets --openai_api_key)"
	export OPENAI_API_KEY
fi

printenv OPENAI_API_KEY | codex login --with-api-key

cd /home/root/work
exec /usr/local/bin/codex-wrapper "$@"
