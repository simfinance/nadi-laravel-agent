#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cd "$ROOT_DIR"

if [[ ! -f "resources/agent/SHA256SUMS.txt" ]]; then
    echo "Missing resources/agent/SHA256SUMS.txt" >&2
    exit 1
fi

sha256sum -c resources/agent/SHA256SUMS.txt

for binary in \
    resources/agent/linux-amd64/nadi-agent \
    resources/agent/linux-arm64/nadi-agent
do
    if [[ ! -x "$binary" ]]; then
        echo "Agent binary is missing or not executable: $binary" >&2
        exit 1
    fi
done

printf '%s\n' "NADI Agent package assets verified."
