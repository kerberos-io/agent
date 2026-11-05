#!/usr/bin/env bash
set -euo pipefail
# Public OCI index pins the platform-specific executable without GitHub API access.
version=0.76.1
image=ghcr.io/aquasecurity/trivy:0.76.1@sha256:790fd66d1697395d8693d1acaa1ea702406a20f109b6fc7bac8db623eca6c06c
evidence_dir="${1:-evidence/scanner-install}"
mkdir -p "$evidence_dir" .ci-tools
stage=host-platform
platform=
cid=
finish() {
  result=$?
  trap - EXIT
  if [[ -n "$cid" ]]; then
    docker rm -f "$cid" >/dev/null 2>&1 || true
  fi
  rm -f .ci-tools/trivy.new
  python3 - "$evidence_dir/install.json" "$result" "$stage" "$image" "$platform" "$version" <<'PY'
import hashlib
import json
from pathlib import Path
import sys
path, code, stage, image, platform, version = sys.argv[1:]
record = {
    "status": "installed" if code == "0" else "failed",
    "exit_code": int(code), "stage": stage, "image": image, "platform": platform,
    "binary_source": "/usr/local/bin/trivy", "expected_version": version,
}
if code == "0":
    record["binary_sha256"] = hashlib.sha256(Path(".ci-tools/trivy").read_bytes()).hexdigest()
Path(path).write_text(json.dumps(record, indent=2) + "\n")
PY
  exit "$result"
}
trap finish EXIT
case "$(uname -m)" in
  x86_64) platform=linux/amd64 ;;
  aarch64|arm64) platform=linux/arm64 ;;
  *) echo "Unsupported scanner host architecture" >&2; exit 1 ;;
esac
stage=pull
docker pull --platform "$platform" "$image"
stage=create
cid=$(docker create --platform "$platform" "$image")
stage=extract
docker cp "$cid:/usr/local/bin/trivy" .ci-tools/trivy.new
chmod 0755 .ci-tools/trivy.new
stage=verify-version
.ci-tools/trivy.new --version | grep -Fx "Version: $version"
mv .ci-tools/trivy.new .ci-tools/trivy
stage=publish-path
if [[ -n "${GITHUB_PATH:-}" ]]; then
  echo "$PWD/.ci-tools" >> "$GITHUB_PATH"
fi
stage=complete
