#!/usr/bin/env bash
set -euo pipefail

mkdir -p /conductor/data /conductor/run

# /conductor is a host bind mount in deployment, so image-owned resources would
# otherwise be hidden. Refresh them on every container start so workers receive
# the resources that match the deployed conductor image.
rm -rf /conductor/worker-resources
cp -a /usr/local/share/conductor/worker-resources /conductor/worker-resources

if [ ! -f /conductor/data/scheduler.sqlite ]; then
	sqlite3 /conductor/data/scheduler.sqlite < /usr/local/share/conductor/database.sql
fi

TMUX_SESSION_NAME="conductor-tmux-session"
TMUX_EXIT_STATUS_FILE="/tmp/conductor-tmux-exit-status"
FIFO_PATH="/tmp/conductor-tmux-output.fifo"

rm -f "$TMUX_EXIT_STATUS_FILE" "$FIFO_PATH"

printf -v CONDUCTOR_COMMAND "%q " /usr/local/bin/conductor "$@"
printf -v QUOTED_EXIT_STATUS_FILE "%q" "$TMUX_EXIT_STATUS_FILE"
TMUX_SHELL_COMMAND="${CONDUCTOR_COMMAND}; status=\$?; printf '%s\n' \"\$status\" > ${QUOTED_EXIT_STATUS_FILE}"

terminate_conductor_tmux() {
	tmux kill-session -t "$TMUX_SESSION_NAME" 2>/dev/null || true
}
trap terminate_conductor_tmux TERM INT

TMUX_PANE_ID="$(tmux new-session -d -s "$TMUX_SESSION_NAME" -P -F "#{pane_id}" "$TMUX_SHELL_COMMAND")"

mkfifo -m 0600 "$FIFO_PATH"

# tmux detaches the conductor from Docker's normal stdout. pipe-pane writes the
# pane output into this FIFO so the entrypoint can keep forwarding logs.
tmux pipe-pane -o -t "$TMUX_PANE_ID" "cat > $FIFO_PATH"

cat "$FIFO_PATH" &
CAT_PID="$!"

while tmux has-session -t "$TMUX_SESSION_NAME" 2>/dev/null; do
	sleep 1
done

wait "$CAT_PID" || true
rm -f "$FIFO_PATH"

if [ -f "$TMUX_EXIT_STATUS_FILE" ]; then
	exit "$(cat "$TMUX_EXIT_STATUS_FILE")"
fi

exit 1
