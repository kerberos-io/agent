
FROM kerberos/base:dc12d68 AS build-machinery
LABEL AUTHOR=Kerberos.io

ENV GOROOT=/usr/local/go
ENV GOPATH=/go
ENV PATH=$GOPATH/bin:$GOROOT/bin:/usr/local/lib:$PATH
ENV GOSUMDB=off

##########################################
# Installing some additional dependencies.

RUN apt-get upgrade -y && apt-get update && apt-get install -y --no-install-recommends \
	git build-essential cmake pkg-config unzip libgtk2.0-dev \
	curl ca-certificates libcurl4-openssl-dev libssl-dev libjpeg62-turbo-dev && \
	rm -rf /var/lib/apt/lists/*

##############################################################################
# Copy all the relevant source code in the Docker image, so we can build this.

RUN mkdir -p /go/src/github.com/kerberos-io/agent
COPY machinery /go/src/github.com/kerberos-io/agent/machinery

##################################################################
# Get the latest commit hash, so we know which version we're running
COPY .git /go/src/github.com/kerberos-io/agent/.git
RUN cd /go/src/github.com/kerberos-io/agent/.git && git log --format="%H" -n 1 | head -c7 > /go/src/github.com/kerberos-io/agent/machinery/version
RUN cat /go/src/github.com/kerberos-io/agent/machinery/version

##################
# Build Machinery

RUN cd /go/src/github.com/kerberos-io/agent/machinery && \
	go mod download && \
	go build -tags timetzdata,netgo,osusergo --ldflags '-s -w -extldflags "-static -latomic"' main.go && \
	mkdir -p /agent && \
	mv main /agent && \
	mv version /agent && \
	mv data /agent && \
	mkdir -p /agent/data/cloud && \
	mkdir -p /agent/data/snapshots && \
	mkdir -p /agent/data/log && \
	mkdir -p /agent/data/recordings && \
	mkdir -p /agent/data/capture-test && \
	mkdir -p /agent/data/config && \
	rm -rf /go/src/gitlab.com/

####################################
# Let's create a /dist folder containing just the files necessary for runtime.
# Later, it will be copied as the / (root) of the output image.

WORKDIR /dist
RUN cp -r /agent ./

####################################################################################
# This will collect dependent libraries so they're later copied to the final image.

RUN /dist/agent/main version

###############################################
# Build Bento4 -> we want fragmented mp4 files

ENV BENTO4_VERSION 1.6.0-639
RUN cd /tmp && git clone https://github.com/axiomatic-systems/Bento4 && cd Bento4 && \
	git checkout tags/v${BENTO4_VERSION} && \
	cd Build && \
	cmake -DCMAKE_BUILD_TYPE=Release .. && \
	make && \
	mv /tmp/Bento4/Build/mp4fragment /dist/agent/ && \
	rm -rf /tmp/Bento4

FROM node:18.14.0-alpine3.16 AS build-ui

RUN apk update && apk upgrade --available && sync

########################
# Build Web (React app)

RUN mkdir -p /go/src/github.com/kerberos-io/agent/machinery/www
COPY ui /go/src/github.com/kerberos-io/agent/ui
RUN cd /go/src/github.com/kerberos-io/agent/ui && rm -rf yarn.lock && yarn config set network-timeout 300000 && \
	yarn && yarn build

####################################
# Let's create a /dist folder containing just the files necessary for runtime.
# Later, it will be copied as the / (root) of the output image.

WORKDIR /dist
RUN mkdir -p ./agent && cp -r /go/src/github.com/kerberos-io/agent/machinery/www ./agent/

############################################
# Publish main binary to GitHub release

FROM alpine:latest

############################
# Protect by non-root user.

RUN addgroup -S kerberosio && adduser -S agent -G kerberosio && addgroup agent video

#################################
# Copy files from previous images

COPY --chown=0:0 --from=build-machinery /dist /
COPY --chown=0:0 --from=build-ui /dist /

RUN apk update && apk add ca-certificates curl libstdc++ libc6-compat --no-cache && rm -rf /var/cache/apk/*

##################
# Try running agent

RUN mv /agent/* /home/agent/
RUN cp /home/agent/mp4fragment /usr/local/bin/
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
RUN apk add libcap && setcap 'cap_net_bind_service=+ep' /home/agent/main

###################
# Run non-root user

USER agent

######################################
# By default the app runs on port 80

EXPOSE 80

######################################
# Check if agent is still running

HEALTHCHECK CMD curl --fail http://localhost:80 || exit 1   

###################################################
# Leeeeettttt'ssss goooooo!!!
# Run the shizzle from the right working directory.
WORKDIR /home/agent
CMD ["./main", "-action", "run", "-port", "80"]
