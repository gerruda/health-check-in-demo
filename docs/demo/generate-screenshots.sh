#!/usr/bin/env bash
# Перегенерация демо-скриншотов. Нужен google-chrome/chromium.
set -euo pipefail
cd "$(dirname "$0")"
CHROME="${CHROME:-google-chrome}"
if ! command -v "$CHROME" >/dev/null 2>&1; then
  CHROME=chromium
fi
command -v "$CHROME" >/dev/null 2>&1 || { echo "нужен google-chrome или chromium" >&2; exit 1; }

declare -A heights=( [01-status]=520 [02-help]=700 [03-checkin]=520 [04-training]=560 [05-nutrition]=500 [06-ask]=700 [07-week]=700 )
for name in "${!heights[@]}"; do
  rm -rf /tmp/healthbot-chrome-demo
  "$CHROME" --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage \
    --hide-scrollbars --user-data-dir=/tmp/healthbot-chrome-demo \
    --window-size="760,${heights[$name]}" \
    --screenshot="assets/${name}.png" "file://$PWD/html/${name}.html" >/dev/null 2>&1
  echo "assets/${name}.png"
done
