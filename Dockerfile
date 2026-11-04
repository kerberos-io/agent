
ARG GO_IMAGE=golang:1.26.10-trixie@sha256:a050dcceee6eb9bf110151662dfac7662f3bec40927c7dee48731e3d92083d42
ARG RUNTIME_IMAGE=debian:trixie-slim@sha256:f6b08b09a05725f9431502d3b5714f5c4576e82c1a84fa1debe0c24a7e0c3ec4
ARG VERSION=0.0.0
FROM ${GO_IMAGE} AS build-machinery
LABEL AUTHOR=uug.ai

# Re-declare VERSION inside this stage so the value passed via
# `--build-arg VERSION=...` (e.g. the release tag) is available below.
# ARGs declared before the first FROM are not visible inside build stages.
ARG VERSION
ARG TARGETARCH

ENV GOROOT=/usr/local/go
ENV GOPATH=/go
ENV PATH=$GOPATH/bin:$GOROOT/bin:/usr/local/lib:$PATH
ENV GOTOOLCHAIN=local
ENV CGO_ENABLED=1

##########################################
# Installing some additional dependencies.

RUN apt-get update && apt-get install -y --no-install-recommends \
	git build-essential cmake pkg-config unzip libgtk2.0-dev \
	curl ca-certificates libavcodec-dev libavutil-dev libcurl4-openssl-dev \
	libssl-dev libjpeg62-turbo-dev libswscale-dev && \
	rm -rf /var/lib/apt/lists/*

##############################################################################
# Copy dependency metadata first so module downloads can be cached separately.

RUN mkdir -p /go/src/github.com/kerberos-io/agent
WORKDIR /go/src/github.com/kerberos-io/agent/machinery
COPY machinery/go.mod machinery/go.sum ./
RUN go version && go mod download && go mod verify

##############################################################################
# Copy the rest of the source after dependencies are primed.

COPY machinery ./

##################
# Build Machinery

RUN RESOLVED_VERSION="${VERSION:-0.0.0}" && \
	printf '%s' "${RESOLVED_VERSION}" > version && \
	BUILD_TAGS=timetzdata,netgo,osusergo && \
	case "${TARGETARCH:-$(go env GOARCH)}" in amd64|arm64) BUILD_TAGS="moq,${BUILD_TAGS}" ;; esac && \
	go build -mod=readonly -trimpath -buildvcs=false -o main -tags "${BUILD_TAGS}" --ldflags "-s -w -X github.com/kerberos-io/agent/machinery/src/utils.VERSION=${RESOLVED_VERSION}" . && \
	mkdir -p /agent && \
	mv main /agent && \
	mv version /agent && \
	mv data /agent && \
	mkdir -p /agent/data/cloud && \
	mkdir -p /agent/data/tus && \
	mkdir -p /agent/data/snapshots && \
	mkdir -p /agent/data/log && \
	mkdir -p /agent/data/recordings && \
	mkdir -p /agent/data/capture-test && \
	mkdir -p /agent/data/config

####################################
# Let's create a /dist folder containing just the files necessary for runtime.
# Later, it will be copied as the / (root) of the output image.

WORKDIR /dist
RUN cp -r /agent ./

####################################################################################
# This will collect dependent libraries so they're later copied to the final image.

RUN /dist/agent/main version

FROM node:22-alpine@sha256:fe4f6d74e20370acb04ad889077d14d0f64a9c8ea6a42bc60cddf1e844699914 AS build-ui

RUN apk update && apk upgrade --available && sync

########################
# Build Web (React app)

RUN mkdir -p /go/src/github.com/kerberos-io/agent/machinery/www
WORKDIR /go/src/github.com/kerberos-io/agent/ui
COPY ui/package.json ui/yarn.lock ./
RUN yarn config set network-timeout 300000 && yarn --frozen-lockfile
COPY ui ./
RUN yarn build

####################################
# Let's create a /dist folder containing just the files necessary for runtime.
# Later, it will be copied as the / (root) of the output image.

WORKDIR /dist
RUN mkdir -p ./agent && cp -r /go/src/github.com/kerberos-io/agent/machinery/www ./agent/

############################################
# Publish main binary to GitHub release

FROM ${RUNTIME_IMAGE} AS runtime-rootfs

RUN apt-get update && apt-get install -y --no-install-recommends \
	ca-certificates curl ffmpeg libatomic1 libcap2-bin libstdc++6 \
	dash coreutils findutils grep sed tzdata ncurses-base && \
	groupadd --system --gid 999 kerberosio && \
	useradd --system --uid 999 --gid kerberosio --groups video \
		--home-dir /home/agent --no-create-home agent

#################################
# Copy files from previous images

COPY --chown=0:0 --from=build-machinery /dist/agent/ /runtime/home/agent/
COPY --chown=0:0 --from=build-ui /dist/agent/ /runtime/home/agent/
COPY deployments/docker/assemble-rootfs.sh /usr/local/bin/assemble-rootfs

RUN chmod 0755 /runtime/home/agent /runtime/home/agent/main && \
	cp /runtime/home/agent/data/config/config.json /runtime/home/agent/data/config.template.json && \
	chown -R agent:kerberosio /runtime/home/agent/data /runtime/home/agent/www && \
	sh /usr/local/bin/assemble-rootfs

FROM scratch

COPY --from=runtime-rootfs /runtime/ /

ENV PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
ENV HOME=/home/agent
ENV XDG_CACHE_HOME=/tmp/agent-cache

###################
# Run non-root user

USER agent

######################################
# By default the app runs on port 80

EXPOSE 80

######################################
# Check if agent is still running

HEALTHCHECK --timeout=10s CMD curl --noproxy '*' --fail --silent --show-error --max-time 5 "http://localhost:${AGENT_PORT:-80}/health" || exit 1

###################################################
# Leeeeettttt'ssss goooooo!!!
# Run the shizzle from the right working directory.
WORKDIR /home/agent
CMD ["./main", "-action", "run"]