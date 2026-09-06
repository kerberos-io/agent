
ARG GO_IMAGE=golang:1.25-trixie
ARG RUNTIME_IMAGE=debian:trixie-slim
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
ENV GOSUMDB=off

##########################################
# Installing some additional dependencies.

RUN apt-get update && apt-get install -y --fix-missing --no-install-recommends \
	git build-essential cmake pkg-config unzip libgtk2.0-dev \
	curl ca-certificates libavcodec-dev libavutil-dev libcurl4-openssl-dev \
	libssl-dev libjpeg62-turbo-dev libswscale-dev && \
	rm -rf /var/lib/apt/lists/*

##############################################################################
# Copy dependency metadata first so module downloads can be cached separately.

RUN mkdir -p /go/src/github.com/kerberos-io/agent
WORKDIR /go/src/github.com/kerberos-io/agent/machinery
COPY machinery/go.mod machinery/go.sum ./
RUN go mod download

##############################################################################
# Copy the rest of the source after dependencies are primed.

COPY machinery ./
RUN rm -rf .env

##################
# Build Machinery

RUN RESOLVED_VERSION="${VERSION:-0.0.0}" && \
	printf '%s' "${RESOLVED_VERSION}" > version && \
	BUILD_TAGS=timetzdata,netgo,osusergo && \
	case "${TARGETARCH:-$(go env GOARCH)}" in amd64|arm64) BUILD_TAGS="moq,${BUILD_TAGS}" ;; esac && \
	go build -tags "${BUILD_TAGS}" --ldflags "-s -w -X github.com/kerberos-io/agent/machinery/src/utils.VERSION=${RESOLVED_VERSION}" main.go && \
	mkdir -p /agent && \
	mv main /agent && \
	mv version /agent && \
	mv data /agent && \
	mkdir -p /agent/data/cloud && \
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

FROM node:22-alpine AS build-ui

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

FROM ${RUNTIME_IMAGE}

############################
# Protect by non-root user.

RUN apt-get update && apt-get install -y --no-install-recommends \
	ca-certificates curl ffmpeg libatomic1 libcap2-bin libstdc++6 && \
	rm -rf /var/lib/apt/lists/* && \
	groupadd --system kerberosio && \
	useradd --system --gid kerberosio --groups video --create-home agent && \
	chmod 0755 /home/agent

#################################
# Copy files from previous images

COPY --chown=0:0 --from=build-machinery /dist /
COPY --chown=0:0 --from=build-ui /dist /

##################
# Try running agent

RUN mv /agent/* /home/agent/
RUN /home/agent/main version

#######################
# Make template config

RUN cp /home/agent/data/config/config.json /home/agent/data/config.template.json

###########################
# Set permissions correctly

RUN chown -R agent:kerberosio /home/agent/data
RUN chown -R agent:kerberosio /home/agent/www

###########################
# Grant the necessary root capabilities to the process trying to bind to the privileged port
RUN setcap 'cap_net_bind_service=+ep' /home/agent/main

###################
# Run non-root user

USER agent

######################################
# By default the app runs on port 80

EXPOSE 80

######################################
# Check if agent is still running

HEALTHCHECK CMD curl --fail "http://localhost:${AGENT_PORT:-80}/health" || exit 1

###################################################
# Leeeeettttt'ssss goooooo!!!
# Run the shizzle from the right working directory.
WORKDIR /home/agent
CMD ["./main", "-action", "run"]