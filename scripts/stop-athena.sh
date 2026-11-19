#!/bin/sh
# Stops a running Athena server on the given address (default 127.0.0.1:3000).
# Only processes named "athena" are stopped, so another service on the port is
# reported instead of being terminated.
set -eu
addr="${1:-127.0.0.1:3000}"
port="${addr##*:}"
pids=$(lsof -nP -t -iTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)
[ -z "$pids" ] && exit 0
for pid in $pids; do
  name=$(ps -o comm= -p "$pid" | sed 's#.*/##')
  if [ "$name" != "athena" ]; then
    echo "Port $port is used by $name (pid $pid), not Athena. Stop it yourself or choose another --addr." >&2
    exit 1
  fi
  kill "$pid"
done
# Wait up to 10 seconds for a graceful shutdown before giving up.
i=0
while lsof -nP -t -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; do
  i=$((i + 1))
  if [ "$i" -gt 20 ]; then
    echo "Athena on port $port did not stop within 10 seconds." >&2
    exit 1
  fi
  sleep 0.5
done
echo "Stopped Athena on port $port."
