FROM kerberos/base:977706d AS builder
LABEL AUTHOR=Kerberos.io

ENV GOROOT=/usr/local/go
ENV GOPATH=/go
ENV PATH=$GOPATH/bin:$GOROOT/bin:/usr/local/lib:$PATH
ENV GOSUMDB=off

##########################################
# Installing some additional dependencies.

RUN apt-get update && apt-get install -y --no-install-recommends \
	git build-essential cmake pkg-config unzip libgtk2.0-dev \
	curl ca-certificates libcurl4-openssl-dev libssl-dev \
	libavcodec-dev libavformat-dev libswscale-dev libtbb2 libtbb-dev \
	libjpeg-dev libpng-dev libtiff-dev libdc1394-22-dev && \
	rm -rf /var/lib/apt/lists/*

##############################################################################
# Copy all the relevant source code in the Docker image, so we can build this.

RUN mkdir -p /go/src/github.com/kerberos-io/agent
COPY machinery /go/src/github.com/kerberos-io/agent/machinery
COPY ui /go/src/github.com/kerberos-io/agent/ui

########################
# Download NPM and Yarns

RUN mkdir /usr/local/nvm
ENV NVM_DIR /usr/local/nvm
ENV NODE_VERSION 14.18.1
RUN curl https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.1/install.sh | bash \
	&& . $NVM_DIR/nvm.sh \
	&& nvm install $NODE_VERSION \
	&& nvm alias default $NODE_VERSION \
	&& nvm use default

ENV NODE_PATH $NVM_DIR/v$NODE_VERSION/lib/node_modules
ENV PATH $NVM_DIR/versions/node/v$NODE_VERSION/bin:$PATH

RUN apt-get update && npm install -g yarn
##################################################################
# Build Web
# this will move the /build directory to ../machinery/www

RUN cd /go/src/github.com/kerberos-io/agent/ui && yarn && yarn build

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

RUN /agent/main version
RUN ldd /agent/main | tr -s '[:blank:]' '\n'
RUN ldd /agent/main | tr -s '[:blank:]' '\n' | grep '^/' | \
	xargs -I % sh -c 'mkdir -p $(dirname ./%); cp % ./%;'

##########################################################
# LDD doesnt always work in docker buildx (no idea why..)
# Therefore we are moving some libraries manually

RUN mkdir -p ./usr/lib

RUN [ -f /lib64/ld-linux-x86-64.so.2 ] && $(mkdir -p lib64 && \
	cp /lib64/ld-linux-x86-64.so.2 lib64/) || echo "nothing to do here x86"

RUN [ -f /lib/ld-linux-aarch64.so.1 ] && $(mkdir -p lib/aarch64-linux-gnu && \
	cp /lib/ld-linux-aarch64.so.1 lib/ && \
	cp /lib/aarch64-linux-gnu/lib* lib/aarch64-linux-gnu/ && \
	cp /usr/lib/aarch64-linux-gnu/libopencv* usr/lib && \
	cp /usr/lib/aarch64-linux-gnu/libstdc* usr/lib && \
	cp /usr/lib/aarch64-linux-gnu/libx264* usr/lib ) || echo "nothing to do here arm64"

RUN [ -f /usr/lib/arm-linux-gnueabihf/vfp/neon/libvpx.so.6 ] && \ 
	$(cp /usr/lib/arm-linux-gnueabihf/vfp/neon/libvpx.so.6 ./usr/lib/) || echo "nothing to do here armv7"

RUN cp -r /usr/local/lib/libavcodec* ./usr/lib && \
	cp -r /usr/local/lib/libavformat* ./usr/lib && \
	cp -r /usr/local/lib/libavfilter* ./usr/lib && \
	cp -r /usr/local/lib/libavutil* ./usr/lib && \
	cp -r /usr/local/lib/libavresample* ./usr/lib && \
	cp -r /usr/local/lib/libavdevice* ./usr/lib && \
	cp -r /usr/local/lib/libswscale* ./usr/lib && \
	cp -r /usr/local/lib/libswresample* ./usr/lib && \
	cp -r /usr/local/lib/libpostproc* ./usr/lib

# As mentioned before, above is really a hack as LDD
# doesn't work always in docker buildx. You might not need this 
# when doing a local build.
################################################################

FROM alpine:latest

############################
# Protect by non-root user.

RUN addgroup -S kerberosio && adduser -S agent -G kerberosio && addgroup agent video

#################################
# Copy files from previous images

COPY --chown=0:0 --from=builder /dist /
COPY --chown=0:0 --from=builder /usr/local/go/lib/time/zoneinfo.zip /zoneinfo.zip

ENV ZONEINFO=/zoneinfo.zip

RUN apk update && apk add ca-certificates --no-cache && \
	apk add tzdata curl --no-cache && rm -rf /var/cache/apk/*

#################
# Install Bento4
RUN cd && wget https://www.bok.net/Bento4/binaries/Bento4-SDK-1-6-0-639.x86_64-unknown-linux.zip && \
	unzip Bento4-SDK-1-6-0-639.x86_64-unknown-linux.zip && rm Bento4-SDK-1-6-0-639.x86_64-unknown-linux.zip && \
	cp ~/Bento4-SDK-1-6-0-639.x86_64-unknown-linux/bin/mp4fragment /usr/bin/

##################
# Try running agent

RUN mv /agent/* /home/agent/
RUN /home/agent/main version

###########################
# Set permissions correctly

RUN chown -R agent:kerberosio /home/agent/data

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
CMD ["./main", "run", "opensource", "80"]
