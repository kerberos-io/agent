# Deployment with Docker

The easiest, and let's say most natural, deployment is done [by utilising `docker`](#1-running-a-single-container). Docker can run a stand-alone, single, Kerberos Agent (or container) and a bigger set of Kerberos Agents (or containers) [through `docker compose`](#2-running-multiple-containers-with-docker-compose).

## 1. Running a single container

We publish production images on [Docker Hub](https://hub.docker.com/r/kerberos/agent).
The following examples target the forthcoming hardened image, which listens on
**8080 inside the container**. They preserve host port 80 and the browser URL
`http://localhost/`:

    docker run -p 80:8080 --name mycamera -d kerberos/agent:latest

**Migration:** releases published before this change, including `v3.14.1`, still
listen on container port 80 and require `-p 80:80`. Do not change their mappings
until upgrading the image too. The separate `kerberos/agent-dev` image is not
migrated by this production-image change:

    docker run -p 80:80 --name mycamera -d kerberos/agent-dev:latest

Feel free to use another port if your host system already has a workload running on `80`. For example `8082`.

    docker run -p 8082:8080 --name mycamera -d kerberos/agent:latest

## Image contract

This contract describes the hardened production Dockerfile, not an already
published release or a claim of full
[image-hardening baseline](https://learn.uug.ai/guides/security/image-hardening/)
compliance. Use a release's actual published multi-platform digest, for example
`kerberos/agent:<version>@sha256:<digest>`, rather than treating mutable `latest`
as immutable. No new release digest or signature is asserted by this PR.

| Property | Contract |
|---|---|
| Identity | Fixed numeric `USER 999:999`, account `agent`, primary group `kerberosio`; not root and no privilege-changing startup. |
| Startup | Working directory `/home/agent`; exec-form default command `./main -action run`. Standalone binary CLI defaults remain unchanged. |
| Inbound HTTP | TCP `8080`, configurable with `AGENT_PORT` (overrides `-port`). Use an unprivileged port. Map host `80:8080`, or Service port `80` to `targetPort: 8080`. Terminate inbound TLS at a trusted proxy; the default listener is HTTP. |
| Privileges | No bundled setuid/setgid programs or file capabilities. Run with all capabilities dropped and `no-new-privileges`; no `NET_BIND_SERVICE` is required. |
| Writable application paths | Only `/home/agent/data/` (configuration, recordings, snapshots, cloud, tus checkpoints, capture tests and logs). Binary, UI assets and application home remain root-owned/read-only to UID 999. Demo mode serves its environment without rewriting UI assets. |
| Temporary state | `/tmp`, `/var/tmp`; user font/media caches under `XDG_CACHE_HOME=/tmp/agent-cache`, owned by `999:999` with mode `0700`. Recreate that directory with these permissions when mounting a fresh tmpfs over `/tmp`. |
| Health | Exec `/home/agent/main -action healthcheck`; five-second bounded localhost `/health` probe on the resolved port, no proxy/redirects, validates the Agent JSON envelope, exits nonzero on failure. Docker timeout is ten seconds. |
| Readiness and metrics | `/health` is HTTP liveness, not proof of camera, MQTT or Hub readiness. Camera/Hub details are diagnostic. This image does not add a Prometheus metrics listener. |
| Certificates/time | Debian CA bundle, OpenSSL configuration and timezone data retained. Mount a prepared CA bundle read-only and use `SSL_CERT_FILE` where appropriate for Go clients; native FFmpeg/OpenSSL clients may require their own CA configuration. Do not disable TLS verification as a substitute. |
| Secrets | Inject application credentials/keys at runtime using the orchestrator's existing environment/configuration mechanisms. Change default credentials before exposing HTTP. Never build production config, `.env`, private keys or registry credentials into an image. No new generic `*_FILE` interface is provided. |
| Outbound traffic | Configured RTSP/RTSPS/ONVIF camera endpoints, MQTT, Hub/Vault/storage HTTPS endpoints, DNS and optional telemetry; WebRTC may require configured STUN/TURN and negotiated UDP/TCP connectivity. Apply an allowlist suited to the actual enabled integrations. |
| Architecture | Native AMD64 and ARM64 build/runtime checks; ARMv7 remains a legacy build target needing its own evidence. ARMv6 is blocked by the default builder manifest; see below. |

### Container permissions and hardening

The final image uses `FROM scratch` with an explicitly assembled Debian/glibc
runtime rather than the whole Debian filesystem. FFmpeg and its required native
libraries/plugins remain for camera/media functionality, including AAC-to-PCMU.
Apt/dpkg executables, account/capability-management tools, curl, and bundled
setuid/setgid permissions are absent. An application subcommand, not shell/curl,
implements the healthcheck.

```sh
docker run --cap-drop=ALL --security-opt=no-new-privileges:true \
  -p 80:8080 --name mycamera -d kerberos/agent:latest
```

Do not use `--privileged` for ordinary IP cameras. Local cameras still use the
separate RTSP sidecar; any device access belongs to that sidecar. Host networking
remains supported, but without a mapping the Agent is reachable on host port
`8080`, not 80.

The numeric `USER 999:999` does **not** automatically inherit supplementary video
membership from `/etc/group`. If a deployment deliberately passes a video device
to Agent, explicitly add the device's host GID: `--group-add 44` for GID 44,
Compose `group_add: ["44"]`, or Kubernetes `supplementalGroups: [44]`. The runtime
retains the `video` group record. Match the actual device permissions; this does
not grant arbitrary device access or replace the separate local-camera sidecar.

**Pending policy exception:** functional remote terminal sessions require
`/bin/sh`, PTY devices, terminal data and basic GNU utilities. These remain for
that feature, not healthchecks or debugging convenience. The user chose to
preserve the feature, but no policy approver, exception owner or expiry has been
provided. This is not an approved waiver or full baseline/CIS compliance.
Baseline-qualified release promotion must remain blocked until the exception is
formally resolved; do not silently remove the feature or manufacture approval.

Bind mounts must be writable by the image's UID/GID; the container does not
change host ownership at startup. Before migrating an older volume, check the
user of the exact old and new image with `docker run --rm --entrypoint id IMAGE`
and adjust only the mounted directories as necessary. For example:

```sh
sudo chown -R 999:999 agent/config agent/recordings
```

A read-only root filesystem is not enabled by default: `data/` needs initialized
writable storage (and FFmpeg or shell sessions may need `/tmp` and `/var/tmp`).
Empty bind mounts hide packaged configuration/media files; initialize
them before use. Docker named volumes populate packaged contents on first use.
Customers enabling read-only root must supply those writable mounts and cache
ownership, keep the runtime's device/PTY mounts, and validate their enabled media
and remote-access features. Apply CPU/memory/PID limits, seccomp/AppArmor and
network policies appropriate to the deployment.

### Building and checking the image

Build from a clean checkout: the default configuration files under
`machinery/data/config/` are included in the image, so do not replace them with
production credentials before building. `.dockerignore` excludes local `.env`
files, credential directories/configuration, test/profiling outputs, recordings,
upload checkpoints, dependency directories, and unrelated
repository files from the build context. The builder uses Go `1.26.10` with
`GOTOOLCHAIN=local`, checksum database verification, and `go mod verify`. Builds
use `-mod=readonly` to reject implicit module changes, `-trimpath` to omit local
source paths, and `-buildvcs=false` to omit checkout metadata. CGO remains enabled
for the native camera/media dependencies.

Source builds require at least Go 1.26.10, matching the pinned security patch floor.
The Go, Node, and Debian bases are pinned to verified multi-platform manifest
digests, including AMD64 and ARM64 (and ARMv7 where previously supported).
The pinned Debian image is used only to assemble the scratch runtime with
glibc-compatible FFmpeg, CGO, and MoQ dependencies. Its native package dependency
metadata selects the architecture's dynamic loader, shared libraries, media
plugins/drivers, and associated data files. This is intentionally broader than
an `ldd`-only copy, which misses dynamically loaded plugins. Certificates,
OpenSSL configuration, time zones, font/MIME/pixbuf caches, account records, and
standard writable `/tmp` and device mount points are retained. The runtime
uses `XDG_CACHE_HOME=/tmp/agent-cache` for disposable user caches without making
the application home writable. It contains package provenance in
`/var/lib/dpkg/status` and license notices, but
not the package manager. Update it by rebuilding and replacing the image rather
than installing packages in a running container.

```sh
docker build --pull -t kerberos/agent:local .
sh deployments/docker/test-image.sh kerberos/agent:local
```

The smoke check uses temporary containers and a Docker volume, cleans them up,
and checks permissions, FFmpeg audio transcoding, H264/sample-video decoding,
H265 encoding/decoding, font rendering, shell/PTY devices, volume
initialization/writes, default and custom HTTP ports, health probes, and
demo-mode startup with a read-only root and immutable UI assets. It enforces
privileged low ports while running on 8080 with no capabilities and
`no-new-privileges`, and also checks an overridden port `8082`. PR builds
run it on native AMD64 and ARM64 runners. It does not replace testing against a
real camera, hardware codecs/devices, an MQTT-mediated remote shell session, or
all supported ARM variants. Review and refresh the
base-image digests regularly for security updates; `--pull` alone does not update
a pinned digest. `GO_IMAGE` and `RUNTIME_IMAGE` remain overrideable build arguments
for compatible alternatives. The package repositories are not snapshot-pinned,
so base digest pins do not make the entire build bit-for-bit reproducible.

Architecture selections are unchanged by this hardening.
The pinned `golang:1.26.10-trixie` manifest, like the previous Go 1.25 trixie base,
has no `linux/arm/v6` variant, so the existing ARMv6 nightly target cannot be
validated with the default base.
ARMv6 needs a separately reviewed compatible builder/runtime combination; do
not assume an ARMv7 image runs on ARMv6 hardware.

### Release evidence, vulnerabilities and lifecycle

The hardened release path must build/test the supported release architectures,
block HIGH/CRITICAL vulnerability/secret/misconfiguration findings, retain scan
reports and an SBOM, publish build provenance, and sign the **final
multi-platform index digest**, not just one child manifest. A pending policy
exception is not an approved scan exception. There are no approved
vulnerability exceptions in this change; findings require remediation or a
reviewed owner/expiry/approval record. Scanner/database failures must fail
closed. Workflow definitions are not evidence that registry publication,
signing, or scans have already succeeded.

Source scans cover UI production **and build/development** dependencies and retain
their application inventory/SBOM, because compiled browser assets hide lockfile
metadata from final-rootfs scanners. Final-image evidence separately covers native
packages and the Go binary. Current native-library findings remain release
blockers. The existing embedded recording-signing key and trust behavior are
retained under the user's time-limited source-secret exception, recorded for
review in this PR in
[image-scan-exceptions.json](../../.github/scripts/image-scan-exceptions.json).
Owner and approver: `@cedricve`; expiry: **December 5, 2026, 23:59:59 UTC**.
Anyone possessing this shared private key can forge signatures under its
identity; the exception accepts that risk temporarily, it does not fix it.

The exception matches only that source file's SHA-256, exact scanner rule,
category, HIGH severity and line location, and requires exactly one match.
Changes, missing/incomplete approval, expiry or duplicate findings fail closed.
It cannot exempt an image finding, another secret, a vulnerability or a
misconfiguration. The policy is loaded from the executing workflow's checkout,
not the source directory being scanned. Retained `scan.json` still contains the
redacted HIGH finding; `exception-policy.json`, `exception-decisions.json` and
`policy.json` record the approval/application, without key material. Review the
record in PR #350 before merging; repository owners must enforce required reviews.
This exception does not approve the separate pending remote-terminal policy.

The build jobs run compilation and production-image smoke tests before the
security gates. A job that fails at **Source/UI dependency and final image gates
with SPDX SBOMs** is not a compiler or startup failure; inspect its retained
reports rather than disabling the gate. Patched Go dependencies are checked
with `govulncheck` as well as the image/source scanner. The pinned
`golang.org/x/crypto` revision is the official reviewed OpenPGP module split
for [GO-2026-5932](https://pkg.go.dev/vuln/GO-2026-5932); Agent does not import the
retired OpenPGP packages.

Debian 13 stable, security and backports currently provide no patched FFmpeg
7.1.x or libxml2 packages for the blocking findings on either AMD64 or ARM64.
For example, Debian defers [CVE-2026-58049](https://security-tracker.debian.org/tracker/CVE-2026-58049)
until an upstream 7.1.x fix; a fix exists in FFmpeg 9. A Wolfi FFmpeg 7 prototype
had no vendor-scanner findings but still used the same affected upstream
7.1.5 code and lacked the existing hardware accelerators. It was **not**
adopted to obtain a misleading clean scan. A newer media stack needs matched
CGO headers/libraries and validation of codecs, hardware acceleration and
supported architectures, not an unreviewed mix of testing-distribution packages.
The user has chosen to keep the default recording-signing key; no trust
migration or startup change is implemented. Its source-secret finding becomes
release-blocking again if the scoped exception expires or no longer matches.
Any future removal would require a separately
authorized migration: per-installation keys need verifier/public-key provisioning,
while requiring an injected key changes startup for existing keyless installations.

Before deploying, obtain the release's digest, source revision, supported
platforms, per-platform SBOM/scan reports, provenance, signature certificate
identity and policy approvals from its retained release evidence. Verify the
signature against the **exact trusted workflow identity** and GitHub OIDC issuer,
not an unrestricted identity regexp:

Workflow artifacts `image-source-evidence` and `image-final-evidence` retain
sanitized reports, source/application and per-platform SBOMs, provenance, base
digests and the source revision for 90 days. Successful releases also attach
`image-security-evidence.tar.gz`. After successful signing,
`signing-identity.txt` records the exact
`https://github.com/kerberos-io/agent/.github/workflows/image-release.yml@...`
identity; the suffix is the executing workflow ref, not necessarily the source
ref. Require the expected trusted ref, not merely any identity found in an
artifact. Secret matches/source excerpts are redacted; findings are not waived.

```sh
# Set these from the trusted release's real evidence, not from this example.
cosign verify --certificate-identity="$COSIGN_IDENTITY" \
  --certificate-oidc-issuer=https://token.actions.githubusercontent.com \
  "$AGENT_IMAGE"
```

`AGENT_IMAGE` must contain the immutable registry digest. Reject missing,
mismatched, expired or unapproved evidence; neither a successful local build nor
`FROM scratch` certifies the image is vulnerability-free. Registry credentials,
OIDC permissions and repository release protections must be configured by the
repository owner; this PR does not invent them or certify those external controls.

Review pinned digests and native/package/module findings for every release and
regularly between releases. Debian 13 has
[full support until August 9, 2028 and LTS until June 30, 2030](https://www.debian.org/releases/trixie/),
with reduced LTS architecture/package coverage. The Go 1.26 line is supported
[until two newer major Go releases exist](https://go.dev/doc/devel/release#policy);
update its patch pin and replace it before that boundary. The build-only Node 22
line has [scheduled EOL April 30, 2027](https://github.com/nodejs/Release/blob/main/schedule.json).
The pinned UI builder is Node `22.23.4` on Alpine `3.24.3`;
[Alpine 3.24 main support ends June 1, 2028](https://alpinelinux.org/releases/),
while community support ends at the next stable release. Track both when
refreshing that build-only base. Rebuilding does not refresh immutable
base pins automatically, and APT repositories are not snapshot-pinned. Retain the
actual installed-package/SBOM evidence per architecture and release.

### Attach a volume

By default your Kerberos agent will store all its configuration and recordings inside the container. It might be interesting to store both configuration and your recordings outside the container, on your local disk. This helps persisting your storage even after you decide to wipe out your Kerberos agent.

You attach a volume to your container by leveraging the `-v` option. To mount your own configuration file, execute as following:

1.  Decide where you would like to store your configuration and recordings; create a new directory for the config file and recordings folder accordingly.

        mkdir agent
        mkdir agent/config
        mkdir agent/recordings

2.  Once you have located your desired directory, copy the latest [`config.json`](https://github.com/kerberos-io/agent/blob/master/machinery/data/config/config.json) file into your config directory.

        wget https://raw.githubusercontent.com/kerberos-io/agent/master/machinery/data/config/config.json -O agent/config/config.json

3.  Run the docker command as following to attach your config directory and recording directory.

        docker run -p 80:8080 --name mycamera \
        -v $(pwd)/agent/config:/home/agent/data/config \
        -v $(pwd)/agent/recordings:/home/agent/data/recordings \
        -d --restart=always kerberos/agent:latest

### Override with environment variables

Next to attaching the configuration file, it is also possible to override the configuration with environment variables. This makes deployments when leveraging `docker compose` or `kubernetes` much easier and more scalable. Using this approach we simplify automation through `ansible` and `terraform`. You'll find [the full list of environment variables on the main README.md file](https://github.com/kerberos-io/agent#override-with-environment-variables).

### 2. Running multiple containers with Docker compose

When running multiple containers, you could execute the above process multiple times, or a better way is to run a `docker compose` with predefined configuration file, a `docker-compose.yaml`.

You'll find [an example `docker-compose.yaml` file here](https://github.com/kerberos-io/agent/blob/master/deployments/docker/docker-compose.yaml). This configuration file includes a definition for running 3 Kerberos Agents (or containers). By specifying environment variables you can override the internal configuration. To add more Kerberos Agents to your deployment, just `copy-paste` a `service` block and modify the name, exposed port, and settings accordingly.

    kerberos-agent2:
        image: "kerberos/agent:latest"
        ports:
        - "8082:8080"
        environment:
        - AGENT_NAME=agent2
        - AGENT_CAPTURE_IPCAMERA_RTSP=rtsp://x.x.x.x:554/Streaming/Channels/101
        - AGENT_HUB_KEY=yyy
        - AGENT_HUB_PRIVATE_KEY=yyy

#### Attaching volumes

As described in [1. Running a single container](#1-running-a-single-container) you can also assign volumes to your `docker compose` services. A volume can be added to persist the recordings of your Kerberos Agents on the host machine, or to provide more accurate configurations.

When attaching a volume for persisting recordings or mounting configuration files from the host system. the `docker-compose.yaml` would look like this.

Let's start by creating some directories on your host system. We'll consider 3 Kerberos Agents in this example.

    mkdir -p agent1/config agent1/recordings
    mkdir -p agent2/config agent2/recordings
    mkdir -p agent3/config agent3/recordings

Download the configuration file in each Kerberos Agent configuration directory.

    wget https://raw.githubusercontent.com/kerberos-io/agent/master/machinery/data/config/config.json -O agent1/config/config.json
    wget https://raw.githubusercontent.com/kerberos-io/agent/master/machinery/data/config/config.json -O agent2/config/config.json
    wget https://raw.githubusercontent.com/kerberos-io/agent/master/machinery/data/config/config.json -O agent3/config/config.json

Next we'll add a `volumes:` section to each Kerberos Agent (service) in the `docker-compose-with-volumes.yaml` file.

    volumes:
      - ./agent1/config:/home/agent/data/config
      - ./agent1/recordings:/home/agent/data/recordings

We'll repeat that for the other Kerberos Agents as well. You can review [the final result over here](https://github.com/kerberos-io/agent/blob/master/deployments/docker/docker-compose-with-volumes.yaml).

Run the `docker compose` command by providing a different configuration file name.

    docker compose -f docker-compose-with-volumes.yaml up

Please note that you can use a combination of using a configuration file and environment variables at the same time. However environment variables will always override the setting in your configuration file.
