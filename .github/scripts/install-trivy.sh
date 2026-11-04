#!/usr/bin/env bash
set -euo pipefail
# Release assets and these platform names were checked against v0.76.1.
version=0.76.1
case "$(uname -m)" in
  x86_64) platform=64bit ;;
  aarch64|arm64) platform=ARM64 ;;
  *) echo "Unsupported scanner host architecture" >&2; exit 1 ;;
esac
mkdir -p .ci-tools
asset="trivy_${version}_Linux-${platform}.tar.gz"
gh release download "v${version}" --repo aquasecurity/trivy \
  --pattern "$asset" --pattern "trivy_${version}_checksums.txt" --dir .ci-tools
(
  cd .ci-tools
  grep "  ${asset}$" "trivy_${version}_checksums.txt" | sha256sum --check --strict -
  tar -xzf "$asset" trivy
)
echo "$PWD/.ci-tools" >> "$GITHUB_PATH"
