#!/usr/bin/env bash

echo '==== Attempting Login ===='

# Logs in and creates a ~/.codex config directory.
printenv OPENAI_API_KEY | codex login --with-api-key

echo '==== Starting Worker ===='

worker
