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
# start ssh server if DEBUG_SSH_ENABLED env var is truthy
# ==============================================================

case "${DEBUG_SSH_ENABLED:-}" in
	1|true|TRUE|yes|YES|on|ON)
		DEBUG_SSH_PUBLIC_KEY_SECRET_NAME="${DEBUG_SSH_PUBLIC_KEY_SECRET_NAME:-debug_public_ssh_key}"
		install -d -m 0700 /root/.ssh
		get-secrets "$DEBUG_SSH_PUBLIC_KEY_SECRET_NAME" > /root/.ssh/authorized_keys
		chmod 0600 /root/.ssh/authorized_keys
		ssh-keygen -A
		install -d -m 0755 /run/sshd
		/usr/sbin/sshd \
			-o PasswordAuthentication=no \
			-o PermitRootLogin=prohibit-password
		;;
esac

# ==============================================================
# start codex-wrapper
# ==============================================================

cd /home/root/work

# starting codex-wrapper in tmux so we can attach to a running session for adhoc observability
TMUX_SESSION_NAME="agent-codex"
TMUX_PANE_ID="$(tmux new-session -d -s "$TMUX_SESSION_NAME" -P -F "#{pane_id}" /usr/local/bin/codex-wrapper "$@")"

FIFO_PATH="/tmp/agent-meta/tmux-agent-codex-${TMUX_PANE_ID#%}.fifo"
rm -f "$FIFO_PATH"
mkfifo -m 0600 "$FIFO_PATH"

# tmux detaches codex-wrapper from normal container stdout; pipe-pane writes the
# pane output into this FIFO so cat can forward it back into ECS/CloudWatch logs.
tmux pipe-pane -o -t "$TMUX_PANE_ID" "cat > $FIFO_PATH"

cat "$FIFO_PATH" &
CAT_PID="$!"

while tmux has-session -t "$TMUX_SESSION_NAME" 2>/dev/null; do
	sleep 1
done

wait "$CAT_PID" || true
rm -f "$FIFO_PATH"
