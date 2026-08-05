#!/usr/bin/env bash
set -euo pipefail

required_glibc="2.38"
current_glibc="$(getconf GNU_LIBC_VERSION | awk '{print $2}')"

if ! dpkg --compare-versions "$current_glibc" ge "$required_glibc"; then
    echo "MoQ requires glibc ${required_glibc}+; this container has ${current_glibc}." >&2
    echo "Rebuild the devcontainer with the Trixie base, then run this check again." >&2
    exit 1
fi

echo "==> Testing MoQ packages (glibc ${current_glibc})"
GOWORK=off go test -tags moq ./src/cloud/livemoq ./src/cloud

binary="${TMPDIR:-/tmp}/agent-moq"
trap 'rm -f "$binary"' EXIT

echo "==> Linking the MoQ Agent"
GOWORK=off go build -tags moq -o "$binary" ./main.go

echo "==> Running the linked binary"
"$binary" -action version
