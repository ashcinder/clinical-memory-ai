#!/usr/bin/env bash
set -euo pipefail
task_root="$(cd "$(dirname "$0")/../../.." && pwd)"
target_os="${TARGET_OS:-linux}"
target_arch="${TARGET_ARCH:-amd64}"
case "$target_os/$target_arch" in
  linux/amd64|linux/arm64|darwin/arm64|darwin/amd64) ;;
  *) echo 'Supported targets: linux or darwin, amd64 or arm64' >&2; exit 1 ;;
esac
output_dir="${1:-$task_root/后端/output/intranet}"
mkdir -p "$output_dir"
output_dir="$(cd "$output_dir" && pwd)"
bundle_name="clinical-memory-intranet-$target_os-$target_arch"
archive="$output_dir/$bundle_name.tar.gz"
[[ ! -e "$archive" ]] || { echo "Refusing to overwrite: $archive" >&2; exit 1; }
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
bundle="$stage/$bundle_name"
mkdir -p "$bundle/后端/bin" "$bundle/前端Web" "$bundle/数据库数据"
cd "$task_root/后端"
for tool in server backup db-info; do
  CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" go build -trimpath -o "$bundle/后端/bin/linji-$tool" "./cmd/$tool"
done
# Explicit allowlist: never bundle .env, SQL dumps, uploads, models or user keys.
for asset in index.html app.js styles.css demo-match.js favicon.svg demo-assets.json demo-files anatomy; do
  cp -R "$task_root/前端Web/$asset" "$bundle/前端Web/"
done
mkdir -p "$bundle/后端/deploy/intranet"
for asset in README.md VERIFICATION.md clinical-memory.env.example clinical-memory.nginx.conf clinical-memory.service build-bundle.sh check.sh; do
  cp "$task_root/后端/deploy/intranet/$asset" "$bundle/后端/deploy/intranet/"
done
cp "$task_root/数据库数据/schema.sql" "$bundle/数据库数据/"
cp "$task_root/LICENSE" "$bundle/"
cp "$task_root/后端/deploy/intranet/README.md" "$bundle/README.md"
cp "$task_root/后端/deploy/intranet/VERIFICATION.md" "$bundle/VERIFICATION.md"
git -C "$task_root" describe --always --dirty > "$bundle/source-revision.txt"
COPYFILE_DISABLE=1 tar -czf "$stage/bundle.tar.gz" -C "$stage" "$bundle_name"
mv "$stage/bundle.tar.gz" "$archive"
echo "$archive"
