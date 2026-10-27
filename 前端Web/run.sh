#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../后端"
exec go run ./cmd/server -web
