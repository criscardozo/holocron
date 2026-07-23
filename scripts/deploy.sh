#!/usr/bin/env bash
# Cross-compile Holocron for the Raspberry Pi (arm64) and deploy the binary.
# The Pi only ever receives the compiled binary — nothing is built there.
#
# Usage: PI=user@raspberrypi ./scripts/deploy.sh
set -euo pipefail

: "${PI:?set PI=user@host, e.g. PI=pi@raspberrypi.local}"

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "==> Generating templates"
go tool templ generate

echo "==> Cross-compiling for linux/arm64"
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
	go build -trimpath -ldflags="-s -w" -o dist/holocron-arm64 ./cmd/holocron

echo "==> Copying binary to $PI"
scp dist/holocron-arm64 "$PI:/tmp/holocron"

echo "==> Installing and restarting on $PI"
ssh "$PI" 'sudo mv /tmp/holocron /usr/local/bin/holocron && sudo systemctl restart holocron && systemctl --no-pager status holocron | head -n 5'

echo "==> Done"
