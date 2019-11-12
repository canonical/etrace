#!/bin/bash -e

go build ./...

for j in $(seq 1 100); do
    out=$(./etrace run --prepare-script "$(pwd)/prepare-chromium.sh" --window-name "New Tab - Chromium" chromium-browser 2>&1)

    chromeStart=$(echo "$out" | grep /usr/lib/chromium-browser/chromium-browser | awk '{print $1}')

    xdgStart=$(echo "$out" | grep /usr/local/sbin/xdg-settings | awk '{print $1}')

    DELAY=$(( xdgStart - chromeStart ))

    echo "$DELAY"
done