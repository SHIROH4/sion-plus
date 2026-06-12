#!/bin/bash
# Sion stop — kill all dev processes
echo "==> Stopping all Sion dev processes..."

kill_by_port() {
  local port=$1
  local pids=$(lsof -ti :$port 2>/dev/null)
  if [ -n "$pids" ]; then
    echo "  Killing port $port (pids: $pids)"
    kill -9 $pids 2>/dev/null
  fi
}

kill_by_port 8080
kill_by_port 5173

echo "  Done."
