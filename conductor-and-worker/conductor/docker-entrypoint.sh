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

exec /usr/local/bin/conductor "$@"
