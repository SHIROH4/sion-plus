#!/bin/bash
# Sion dev — start Go backend + Vite + Electron
set -e
cd "$(dirname "$0")"

# Clean up any previous processes on our ports
./stop.sh 2>/dev/null

echo "==> Starting Go backend (port 8080)..."
go run ./cmd/sion server &
GO_PID=$!

echo "==> Starting Vite + Electron..."
cd frontend
npx vite &
VITE_PID=$!

# Wait for Vite to be ready, then launch Electron
npx wait-on http://localhost:5173 && npx electron . &
ELEC_PID=$!

echo ""
echo "  Go backend : PID $GO_PID  (http://127.0.0.1:8080)"
echo "  Vite       : PID $VITE_PID (http://localhost:5173)"
echo "  Electron   : PID $ELEC_PID"
echo ""
echo "Press Ctrl+C to stop all."

trap "kill $GO_PID $VITE_PID $ELEC_PID 2>/dev/null; exit 0" INT TERM
wait
