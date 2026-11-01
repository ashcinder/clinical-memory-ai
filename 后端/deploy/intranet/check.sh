#!/usr/bin/env bash
set -euo pipefail
base="${1:?Usage: bash check.sh https://HOST/clinical-memory [CA.pem]}"
base="${base%/}"
[[ "$base" == https://* ]] || { echo 'Use the HTTPS entry URL, not the loopback backend URL.' >&2; exit 1; }
curl_args=(--silent --show-error --connect-timeout 10 --max-time 30 --proto '=https')
if [[ -n "${2:-}" ]]; then curl_args+=(--cacert "$2"); fi
scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT
curl --fail "${curl_args[@]}" "$base/api/health" -o "$scratch/health"
[[ "$(cat "$scratch/health")" == *'"status":"ok"'* ]] || { echo 'Health check returned unexpected data.' >&2; exit 1; }
for asset in / /app.js /styles.css /demo-assets.json; do
  curl --fail "${curl_args[@]}" "$base$asset" -o /dev/null
done
# Anonymous requests must not return account data.
status="$(curl "${curl_args[@]}" -o /dev/null -w '%{http_code}' "$base/api/auth/me")"
[[ "$status" == 401 ]] || { echo "Authentication check failed (HTTP $status)." >&2; exit 1; }
echo 'PASS: HTTPS trust, health, Web assets and anonymous-access checks.'
