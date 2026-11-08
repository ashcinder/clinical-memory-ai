#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
# This script downloads engines/models only, and never uploads personal material.
if [[ "$(uname -s)" == Darwin ]] && command -v brew >/dev/null 2>&1; then
  missing=()
  command -v whisper-cli >/dev/null 2>&1 || missing+=(whisper.cpp)
  command -v ffmpeg >/dev/null 2>&1 || missing+=(ffmpeg)
  if ((${#missing[@]})); then HOMEBREW_NO_AUTO_UPDATE=1 brew install "${missing[@]}"; fi
fi
for engine in whisper-cli ffmpeg; do
  command -v "$engine" >/dev/null 2>&1 || { echo "请安装 $engine 后重试（Linux 可使用系统包或官方构建）。"; exit 1; }
done
model_root="${DATA_DIR:-../数据库数据}/models"
mkdir -p "$model_root/tessdata" "$model_root/tools"
if [[ "$(uname -s)" == Darwin ]]; then
  /usr/bin/swiftc tools/local-ocr.swift -O -o "$model_root/tools/linji-ocr"
  /usr/bin/swiftc tools/local-pdf.swift -O -o "$model_root/tools/linji-pdf"
else
  command -v tesseract >/dev/null 2>&1 || { echo "请安装 tesseract 后重试。"; exit 1; }
fi
fetch_model() {
 local destination="$1" source="$2"
 if [[ ! -s "$destination" ]]; then
   curl --fail --location --retry 3 --output "$destination.download" "$source"
   mv "$destination.download" "$destination"
 fi
}
fetch_model "$model_root/tessdata/chi_sim.traineddata" https://raw.githubusercontent.com/tesseract-ocr/tessdata_fast/main/chi_sim.traineddata
fetch_model "$model_root/tessdata/eng.traineddata" https://raw.githubusercontent.com/tesseract-ocr/tessdata_fast/main/eng.traineddata
fetch_model "$model_root/ggml-base.bin" https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-base.bin
echo "本地检索组件已就绪。重启后端后，可在 Web / APP 关闭云端 AI 搜索。"
