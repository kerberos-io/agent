FROM kerberos/base:caeb5bf AS builder
LABEL AUTHOR=Kerberos.io

ENV GOROOT=/usr/local/go
ENV GOPATH=/go
ENV PATH=$GOPATH/bin:$GOROOT/bin:$PATH
ENV GOSUMDB=off

##############################################################################
# Copy all the relevant source code in the Docker image, so we can build this.

RUN mkdir -p /go/src/github.com/kerberos-io/agent
COPY machinery /go/src/github.com/kerberos-io/agent/machinery
COPY ui /go/src/github.com/kerberos-io/agent/ui

########################
# Download NPM and Yarns

RUN apt-get update && apt-get install -y curl && curl -sL https://deb.nodesource.com/setup_14.x | bash - && \
	curl -sS https://dl.yarnpkg.com/debian/pubkey.gpg | apt-key add - && \
	echo "deb https://dl.yarnpkg.com/debian/ stable main" | tee /etc/apt/sources.list.d/yarn.list && \
	apt update && apt install yarn -y

###########
# Build Web

RUN cd /go/src/github.com/kerberos-io/agent/ui && \
	npm install && yarn build
# this will move the /build directory to ../machinery/www

##################
# Build Machinery

RUN cd /go/src/github.com/kerberos-io/agent/machinery && \
	go mod download && \
	go build main.go && \
	mkdir -p /agent && \
	mv main /agent && \
	mv www /agent && \
	mv data /agent && \
	mkdir -p /agent/data/cloud && \
	mkdir -p /agent/data/snapshots && \
	mkdir -p /agent/data/log && \
	mkdir -p /agent/data/recordings && \
	mkdir -p /agent/data/config && \
	rm -rf /go/src/gitlab.com/

####################################
# Let's create a /dist folder containing just the files necessary for runtime.
# Later, it will be copied as the / (root) of the output image.

WORKDIR /dist
RUN cp -r /agent ./

####################################
# This will collect dependent libraries so they're later copied to the final image

RUN /agent/main version
RUN ldd /agent/main | tr -s '[:blank:]' '\n' | grep '^/' | \
	xargs -I % sh -c 'mkdir -p $(dirname ./%); cp % ./%;'

################################
# We need to move the correct ld

RUN [ -f /lib64/ld-linux-x86-64.so.2 ] && $(mkdir -p lib64 && cp /lib64/ld-linux-x86-64.so.2 lib64/) || echo "nothing to do here x86"
RUN [ -f /lib/ld-linux-aarch64.so.1 ] && $(mkdir -p lib && cp /lib/ld-linux-aarch64.so.1 lib/) || echo "nothing to do here arm64"

RUN mkdir -p ./usr/lib
RUN cp -r /usr/local/lib/libavcodec* ./usr/lib && \
	cp -r /usr/local/lib/libavformat* ./usr/lib && \
	cp -r /usr/local/lib/libswscale* ./usr/lib && \
	cp -r /usr/local/lib/libswresample* ./usr/lib

FROM alpine:latest

############################
# Protect by non-root user.

RUN addgroup -S appgroup && adduser -S appuser -G appgroup

#################################
# Copy files from previous images

COPY --chown=0:0 --from=builder /dist /
COPY --chown=0:0 --from=builder /usr/local/go/lib/time/zoneinfo.zip /zoneinfo.zip

ENV ZONEINFO=/zoneinfo.zip

RUN apk update && apk add ca-certificates --no-cache && \
	apk add tzdata --no-cache && apk add curl --no-cache && rm -rf /var/cache/apk/*

#################
# Install Bento4
RUN cd && wget https://www.bok.net/Bento4/binaries/Bento4-SDK-1-6-0-639.x86_64-unknown-linux.zip && \
	unzip Bento4-SDK-1-6-0-639.x86_64-unknown-linux.zip && rm Bento4-SDK-1-6-0-639.x86_64-unknown-linux.zip && \
	cp ~/Bento4-SDK-1-6-0-639.x86_64-unknown-linux/bin/mp4fragment /usr/bin/

##################
# Try running agent

RUN /agent/main version

###################
# Run non-root user

USER appuser

######################################
# By default the app runs on port 8080

EXPOSE 8080

######################################
# Check if agent is still running

HEALTHCHECK CMD curl --fail http://localhost:8080 || exit 1   

###################################################
# Leeeeettttt'ssss goooooo!!!
# Run the shizzle from the right working directory.
WORKDIR /agent
CMD ["/agent/main", "run", "opensource", "8080"]
