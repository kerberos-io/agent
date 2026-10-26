# syntax=docker/dockerfile:1.7
FROM golang:1.26.10-trixie@sha256:909771fef53e3dcb0950255f0c9a8b4728c3b841e61509c7b29b098de7ee02d4 AS build-machinery
ARG VERSION=0.0.0
ARG TARGETARCH
ENV GOTOOLCHAIN=local CGO_ENABLED=1
RUN apt-get update && apt-get install -y --no-install-recommends \
    pkg-config libavcodec-dev libavutil-dev libswscale-dev && \
    rm -rf /var/lib/apt/lists/*
WORKDIR /src/machinery
COPY machinery/go.mod machinery/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    go mod download && go mod verify
COPY machinery/*.go ./
COPY machinery/docs ./docs
COPY machinery/src ./src
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    BUILD_TAGS=timetzdata,netgo,osusergo && \
    case "${TARGETARCH:-$(go env GOARCH)}" in amd64|arm64) BUILD_TAGS="moq,${BUILD_TAGS}" ;; esac && \
    go build -mod=readonly -trimpath -buildvcs=false -tags "${BUILD_TAGS}" \
      -ldflags="-w -buildid= -X github.com/kerberos-io/agent/machinery/src/utils.VERSION=${VERSION}" \
      -o /out/main . && \
    printf '%s' "${VERSION}" > /out/version && /out/main -action version

FROM build-machinery AS test
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    BUILD_TAGS=timetzdata,netgo,osusergo && \
    case "${TARGETARCH:-$(go env GOARCH)}" in amd64|arm64) BUILD_TAGS="moq,${BUILD_TAGS}" ;; esac && \
    go vet -tags "${BUILD_TAGS}" ./... && go test -p=1 -tags "${BUILD_TAGS}" ./... && \
    go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -tags "${BUILD_TAGS}" ./... && \
    go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -mode=binary /out/main

FROM node:22-bookworm-slim@sha256:de170b39439eb0e5a3444ad7d60b015614a381e10fecae2e30ec9e735122f38d AS build-ui
WORKDIR /src/ui
COPY ui/package.json ui/yarn.lock ./
RUN yarn install --frozen-lockfile --non-interactive
COPY ui/public ./public
COPY ui/src ./src
COPY ui/.eslintrc.json ui/.prettierrc.json ./
RUN yarn build && test -s /src/machinery/www/index.html

# Native FFmpeg capture/transcoding and the opt-in remote terminal require
# glibc, codecs and /bin/sh. Keep this media exception, not a scratch image.
FROM debian:trixie-slim@sha256:62bd423aeee912639ca8ee681c2b762cb3e25137e0a92b20be455898a9f034ad AS runtime-files
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl ffmpeg libatomic1 libstdc++6 && \
    rm -rf /var/lib/apt/lists/* /var/cache/apt/* && \
    groupadd --gid 65532 kerberosio && \
    useradd --uid 65532 --gid 65532 --groups video --home-dir /home/agent \
      --no-create-home --shell /bin/sh agent && \
    find / -xdev -type f -perm /6000 -exec chmod a-s {} + && \
    rm -f /usr/bin/apt /usr/bin/apt-* /usr/bin/dpkg /usr/bin/dpkg-* \
      /usr/sbin/dpkg-* /usr/sbin/useradd /usr/sbin/groupadd \
      /usr/sbin/usermod /usr/sbin/userdel /usr/sbin/groupmod /usr/sbin/groupdel && \
    rm -rf /usr/lib/apt /usr/lib/dpkg && \
    install -d -o 0 -g 0 -m 0755 /home/agent && \
    install -d -o 65532 -g 65532 -m 0750 \
      /home/agent/data /home/agent/data/cloud /home/agent/data/snapshots \
      /home/agent/data/log /home/agent/data/recordings /home/agent/data/capture-test \
      /home/agent/data/config /home/agent/data/tus
COPY --chown=65532:65532 --chmod=0640 machinery/data/config/config.json machinery/data/config/user.json /home/agent/data/config/
COPY --chown=0:0 --chmod=0444 machinery/data/config/config.json /home/agent/data/config.template.json
COPY --chown=0:0 --chmod=0444 machinery/data/test-480p.mp4 /home/agent/data/test-480p.mp4

FROM runtime-files
ARG VERSION=0.0.0
ARG REVISION=unknown
ARG CREATED
LABEL org.opencontainers.image.title="agent" \
      org.opencontainers.image.description="Kerberos edge camera and media agent" \
      org.opencontainers.image.source="https://github.com/kerberos-io/agent" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.created="${CREATED}" \
      org.opencontainers.image.vendor="UUG.AI"
COPY --from=build-machinery --chown=0:0 --chmod=0555 /out/main /home/agent/main
COPY --from=build-machinery --chown=0:0 --chmod=0444 /out/version /home/agent/version
COPY --from=build-ui --chown=0:0 /src/machinery/www /home/agent/www
COPY --chown=0:0 --chmod=0444 ui/package.json ui/yarn.lock /usr/share/agent-ui/
USER 65532:65532
WORKDIR /home/agent
EXPOSE 80/tcp
HEALTHCHECK --interval=30s --timeout=5s --start-period=30s \
    CMD curl --fail --silent "http://localhost:${AGENT_PORT:-80}/health" || exit 1
CMD ["./main", "-action", "run"]
