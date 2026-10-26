# Security Policy

## Supported Versions

We only provide security fixes for the latest release series on the `master` branch.

## Reporting a Vulnerability

Please do **not** open a public GitHub issue for potential security vulnerabilities.

Use one of the private channels below:

1. Preferred: GitHub private vulnerability reporting
   - https://github.com/kerberos-io/agent/security/advisories/new
2. Fallback: Email
   - support@kerberos.io
   - Optional CC: support@uug.ai

Please include:

- A short summary and impact.
- Reproduction steps or proof of concept.
- Affected version(s), commit hash, or deployment details.
- Any proposed mitigation/workaround.
- Your preferred attribution name.

For faster triage, use this subject format in email:

`[Security][Kerberos Agent] <short title>`

## Response Expectations

- Acknowledgement target: within 3 business days.
- Triage/update target: within 7 business days after acknowledgement.

If you do not receive a response in time, please resend your report and include your original timestamp.

## Disclosure and Credits

We follow coordinated disclosure. After a fix is available, we will credit reporters unless they prefer to stay anonymous.

## Production container contract

The image follows the [image hardening guide](https://learn.uug.ai/guides/security/image-hardening/)
with explicit media/device exceptions. A Dockerfile change alone is not evidence
that an image is hardened: native architecture builds, functional smoke tests,
vulnerability reports and an SBOM must pass before release.

- Build and runtime bases are pinned to verified multi-platform digests. Refresh
  them regularly; pins do not deliver security updates automatically.
- Public Go modules retain checksum verification. No build token is needed.
  Building machinery requires Go 1.26 or newer; CI uses Go 1.26.10.
  Go retains CGO for FFmpeg capture and uses `-w -buildid=` without `-s`, preserving
  symbols for binary vulnerability analysis.
- The fixed runtime identity is `65532:65532`; `/home/agent/main`, `version` and
  `www` are root-owned and not writable by the process. Demo UI configuration is
  served without renaming or modifying packaged assets.
- `/home/agent/data` remains the persistent writable path, including `config`,
  `cloud`, `snapshots`, `log`, `recordings`, `capture-test` and `tus`. Existing
  bind mounts/PVCs must grant UID/GID 65532 access before rollout. Do not change
  their paths or discard existing configuration/recordings.
- The existing `./main -action run` command, `/health`, UI and port **80** remain.
  The executable has no file capabilities. For port 80 without capabilities,
  set `net.ipv4.ip_unprivileged_port_start=0` in the container network namespace
  (Docker normally defaults to this). Alternatively set `AGENT_PORT=8080` and
  update the service/container port mapping. Kubernetes policy must permit the
  chosen sysctl, or use the unprivileged port override.
- Preserve required camera/audio device mappings and add the host device's
  numeric supplemental GID (`--group-add` or Kubernetes `supplementalGroups`).
  Do not use privileged mode or make device nodes world-writable. The image
  retains its `video` group; host device GIDs are not portable.
- Debian's glibc and FFmpeg codecs are required by native capture and media
  processing. `/bin/sh` is retained for the product's optional MQTT remote
  terminal, and curl for its existing healthcheck. This is a documented
  media/remote-terminal exception to the shell-free distroless preference, not
  a general permission to add debugging tools. Package-manager executables,
  compiler toolchains and special setuid/setgid permission bits are removed.
  Do not claim complete deployment CIS compliance.

### Verification and release gates

PR, release and nightly jobs build the `test` target (Go tests/vet plus source
and binary govulncheck), then verify the exact image before it can be published.
The local `verify-image` action runs `image/smoke.sh`, records all severities,
blocks HIGH/CRITICAL findings including unfixed findings, and uploads an SPDX
SBOM. There is no blanket ignore list or non-blocking release scan.

The smoke uses `--read-only --cap-drop ALL --security-opt no-new-privileges`,
a Docker volume initialized from the packaged data defaults, and the low-port
sysctl. It checks default production startup, demo mode with `AGENT_PORT=8080`,
health/UI/config responses, writable data, immutable assets, FFmpeg frame
extraction and ffprobe. Hardware/RTSP camera and MQTT remote-terminal integration
still require representative device-backed validation; synthetic media is not
proof of hardware compatibility.

Run on a native Docker host:

```sh
docker build --target test .
docker build -t agent:validation .
bash image/smoke.sh agent:validation
```

Shared workflow integration remains a release prerequisite: the shared
`worker`/`frontend` profiles do **not** match this image. An Agent profile must
preserve `/home/agent`, its command/port/data/device contract, allow the explicit
media/shell/healthcheck exceptions, publish to `kerberos/agent`, and retain the
downloadable release tar assets. Centralized digest-bound signing, provenance
and published-image verification must be integrated before treating this
rollout as fully validated. Do not substitute the incompatible worker profile.
