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

Native release validation targets AMD64 and ARM64. The legacy nightly ARMv6
matrix is currently blocked: the pinned Go and Node upstream indexes provide
ARMv7, not ARMv6 variants. The complete nightly manifest must therefore remain
blocked rather than silently publishing an incomplete architecture set. ARMv6
needs an independently validated cross-build/media-runtime strategy before its
support can be claimed; do not resolve the failure by weakening the gates.

### Current native validation blocker

The ARM64 build and read-only/cap-drop production, demo and FFmpeg smoke passed
in [CI run 40410591639](https://github.com/kerberos-io/agent/actions/runs/40410591639).
The strict vulnerability gate then failed: the refreshed Debian 13.7 runtime
reported 212 HIGH/CRITICAL package findings, including FFmpeg 7.1.5 and curl
8.14.1, with no fixed Debian versions reported. The run uploaded the complete
inventory and SPDX SBOM in `agent-image-evidence-ARM64-1`. This is **not** a
security-clean image or permission to ship it; resolving these native-library
findings without dropping required capture/codecs remains a rollout blocker.

### Supported-base alternative investigation (2026-10-26)

Compared the current **Debian 13 Trixie** runtime against **Debian 12.15
Bookworm LTS**, using the [Debian security tracker JSON](https://security-tracker.debian.org/tracker/data/json)
and the exact 44 distinct HIGH/CRITICAL CVEs from the native image report.
[Bookworm remains supported until 2028-06-30](https://www.debian.org/releases/bookworm/)
on its listed LTS architectures. Its current FFmpeg is
`7:5.1.9-0+deb12u1`, curl is `7.88.1-10+deb12u15`, and libxml2 is
`2.9.14+dfsg-1.3~deb12u6`.

This supported alternative does **not** clear the gate: **42 of the 44 CVEs
remain open**, including the critical libxml2 `CVE-2026-6653`, both curl CVEs,
and 15 of the 16 FFmpeg CVEs. Bookworm is marked unaffected (`fixed_version=0`)
for FFmpeg `CVE-2026-66041` and libxml2 `CVE-2026-86139`; that improvement is
not sufficient to release.

Bookworm is also not a drop-in runtime replacement: this CGO binary is linked
to FFmpeg 7's `libavcodec.so.61`/`libavutil.so.59`, while Bookworm's FFmpeg 5
uses different SONAMEs (`libavcodec.so.59`/`libavutil.so.57`). Both the Go/native
builder and runtime would require a matched rebuild and codec/device regression
validation. No Bookworm image was built or claimed compatible after the
security metadata already demonstrated the unresolved blockers. Bookworm LTS
also no longer covers armel; it is not a silent solution for the retained ARMv6
requirement.

The tracker reports FFmpeg fixes in `7:9.0.2-1`, curl fixes in
`8.21.0~rc2-1`/`8.21.0~rc3-1`, and the critical libxml2 fix in
`2.14.5+dfsg-0.1` in **Forky**. However,
[Forky is testing, without timely Security Team support](https://www.debian.org/releases/forky/),
not a supported stable-base upgrade; cjson, libsndfile, libX11 and libXrender
blockers also remain open there. It was not substituted into production.
Dropping unused administration tools alone cannot resolve the required FFmpeg
decoder/parser findings. No codec, device integration, remote terminal or
ARMv6 target was removed to obtain a clean scan.

The required decision is whether to wait for supported-distribution fixes or
approve an explicitly maintained native-package/backport source or supported
media image, with ownership for security updates and validation across the
retained architectures. Such a change needs fresh matching native inventory,
image scans and camera/codec/device tests; it is not authorization to ignore
these findings or switch to an unsupported testing distribution.

#### Exact blocking native CVEs

These are the source components behind the 212 package findings in Debian 13.7;
several FFmpeg binaries/libraries share the same source advisories:

| Source component | CVEs |
| --- | --- |
| util-linux | CVE-2026-76642, CVE-2026-78408, CVE-2026-78409, CVE-2026-78410 |
| curl | CVE-2026-12064, CVE-2026-8458 |
| ffmpeg | CVE-2026-58049, CVE-2026-64830, CVE-2026-64832, CVE-2026-64833, CVE-2026-64834, CVE-2026-64835, CVE-2026-66036, CVE-2026-66039, CVE-2026-66040, CVE-2026-66041, CVE-2026-70628, CVE-2026-70632, CVE-2026-75142, CVE-2026-75143, CVE-2026-75144, CVE-2026-75146 |
| acl | CVE-2026-54369 |
| cjson | CVE-2026-16554, CVE-2026-29036, CVE-2026-67215, CVE-2026-67216, CVE-2026-87933 |
| ncurses | CVE-2025-69720 |
| libsndfile | CVE-2026-37555 |
| systemd | CVE-2026-16742 |
| tiff | CVE-2026-36849, CVE-2026-52490 |
| libX11 | CVE-2026-88806 |
| libxml2 | CVE-2026-6653 (CRITICAL), CVE-2026-74860, CVE-2026-86138, CVE-2026-86139, CVE-2026-86140, CVE-2026-86142, CVE-2026-86143, CVE-2026-86144 |
| libXrender | CVE-2026-88807 |
| perl | CVE-2026-9538 |
