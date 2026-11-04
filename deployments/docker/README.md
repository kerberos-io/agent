# Deployment with Docker

The easiest, and let's say most natural, deployment is done [by utilising `docker`](#1-running-a-single-container). Docker can run a stand-alone, single, Kerberos Agent (or container) and a bigger set of Kerberos Agents (or containers) [through `docker compose`](#2-running-multiple-containers-with-docker-compose).

## 1. Running a single container

We are creating Docker images as part of our CI/CD process. You'll find our Docker images on [Docker hub](https://hub.docker.com/r/kerberos/agent). Pick a specific tag of choice, or use latest. Once done run below command, this will open the web interface of your Kerberos agent on port 80.

    docker run -p 80:80 --name mycamera -d kerberos/agent:latest

Or for a develop build:

    docker run -p 80:80 --name mycamera -d kerberos/agent-dev:latest

Feel free to use another port if your host system already has a workload running on `80`. For example `8082`.

    docker run -p 8082:80 --name mycamera -d kerberos/agent:latest

### Container permissions and hardening

The production image runs as `agent` (UID `999`, primary GID `999`), with the
supplementary `video` group retained. The binary and home directory are owned by
root; within `/home/agent`, only `data/` and `www/` are writable by the Agent. `www/` must remain writable
because demo mode replaces `env.js` at startup. Existing `/home/agent/data/...`
mount paths, the default port `80`, and the startup command are unchanged.

FFmpeg is required for AAC-to-PCMU audio transcoding, curl for the health check,
and `/bin/sh` for remote-access sessions. They and the basic GNU shell utilities
are intentionally retained. The final image uses `FROM scratch`, populated with
an explicitly assembled Debian/glibc runtime rather than the whole Debian
filesystem. Apt, dpkg executables, account-management and capability-management
tools, and setuid/setgid permissions on bundled utilities are absent.
The Agent binary retains only the file capability
`NET_BIND_SERVICE`, needed for port `80` on hosts with privileged low ports.
For an RTSP-only deployment, you can drop the other container capabilities:

```sh
docker run --cap-drop=ALL --cap-add=NET_BIND_SERVICE \
  -p 80:80 --name mycamera -d kerberos/agent:latest
```

Do not use `--privileged` for ordinary IP cameras. Local cameras still use the
separate RTSP sidecar; any device access belongs to that sidecar. Host networking
remains supported. `no-new-privileges` prevents gaining the binary's file
capability: if enabling it, use an unprivileged internal port (`AGENT_PORT=8082`)
and ensure the container's `net.ipv4.ip_unprivileged_port_start` permits that
port. Do not assume port `80` works with that option on every host.

Bind mounts must be writable by the image's UID/GID; the container does not
change host ownership at startup. Before migrating an older volume, check the
user of the exact old and new image with `docker run --rm --entrypoint id IMAGE`
and adjust only the mounted directories as necessary. For example:

```sh
sudo chown -R 999:999 agent/config agent/recordings
```

A read-only root filesystem is not enabled by default: both `data/` and `www/`
need writable storage (and FFmpeg or shell sessions may need `/tmp`).
Change the default application credentials before exposing the service.

### Building and checking the image

Build from a clean checkout: the default configuration files under
`machinery/data/config/` are included in the image, so do not replace them with
production credentials before building. `.dockerignore` excludes local `.env`
files, recordings, upload checkpoints, dependency directories, and unrelated
repository files from the build context. The builder uses Go `1.26.10` with
`GOTOOLCHAIN=local`, checksum database verification, and `go mod verify`. Builds
use `-mod=readonly` to reject implicit module changes, `-trimpath` to omit local
source paths, and `-buildvcs=false` to omit checkout metadata. CGO remains enabled
for the native camera/media dependencies.

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
demo-mode startup. It enforces
privileged low ports to check the file capability rather than relying on
Docker's usual low-port exemption, and checks `no-new-privileges` on port `8082`. PR builds
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

        docker run -p 80:80 --name mycamera \
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
        - "8082:80"
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
