FROM kerberos/debian-opencv-ffmpeg AS builder

ENV GOROOT=/usr/local/go
ENV GOPATH=/go
ENV PATH=$GOPATH/bin:$GOROOT/bin:$PATH
ENV GOSUMDB=off

##############################################################################
# Copy all the relevant source code in the Docker image, so we can build this.

RUN mkdir -p /go/src/github.com/kerberos-io/opensource
COPY machinery /go/src/github.com/kerberos-io/opensource/machinery
COPY web /go/src/github.com/kerberos-io/opensource/web

########################
# Download NPM and Yarns

RUN apt-get update && apt-get install -y curl && curl -sL https://deb.nodesource.com/setup_14.x | bash - && \
    curl -sS https://dl.yarnpkg.com/debian/pubkey.gpg | apt-key add - && \
    echo "deb https://dl.yarnpkg.com/debian/ stable main" | tee /etc/apt/sources.list.d/yarn.list && \
    apt update && apt install yarn -y

###########
# Build Web

RUN cd /go/src/github.com/kerberos-io/opensource/web && \
    npm install && npx browserslist@latest --update-db && yarn build
    # this will move the /build directory to ../machinery/www

##################
# Build Machinery

RUN cd /go/src/github.com/kerberos-io/opensource/machinery && \
   go mod download && \
   go build main.go && \
	 mkdir -p /opensource && \
	 mv main /opensource && \
	 mv www /opensource && \
	 mkdir -p /opensource/data/cloud && \
	 mkdir -p /opensource/data/snapshots && \
	 mkdir -p /opensource/data/log && \
	 mkdir -p /opensource/data/recordings && \
	 mkdir -p /opensource/data/config && \
	 rm -rf /go/src/gitlab.com/

 ####################################
 # Let's create a /dist folder containing just the files necessary for runtime.
 # Later, it will be copied as the / (root) of the output image.

 WORKDIR /dist
 RUN cp -r /opensource ./

 ####################################
 # This will collect dependent libraries so they're later copied to the final image

 RUN ldd /opensource/main | tr -s '[:blank:]' '\n' | grep '^/' | \
     xargs -I % sh -c 'mkdir -p $(dirname ./%); cp % ./%;'
 RUN mkdir -p lib64 && cp /lib64/ld-linux-x86-64.so.2 lib64/
 RUN mkdir -p ./usr/lib
 RUN cp -r /usr/local/lib/libavcodec* ./usr/lib && \
 		 cp -r /usr/local/lib/libavformat* ./usr/lib && \
 		 cp -r /usr/local/lib/libswscale* ./usr/lib && \
		 cp -r /usr/local/lib/libswresample* ./usr/lib
 RUN ldd /opensource/main

 FROM alpine:latest

 #################################
 # Copy files from previous images

 COPY --chown=0:0 --from=builder /dist /
 COPY --chown=0:0 --from=builder /usr/local/go/lib/time/zoneinfo.zip /zoneinfo.zip

 ENV ZONEINFO=/zoneinfo.zip

 RUN apk update && apk add ca-certificates && \
     apk add --no-cache tzdata && rm -rf /var/cache/apk/*

 ####################################
 # ADD supervisor and STARTUP script
 # NOTE: actually this is not needed, as we could simply run a single binary.

 RUN apk add supervisor && mkdir -p /var/log/supervisor/
 ADD ./scripts/supervisor.conf /etc/supervisord.conf
 ADD ./scripts/run.sh /run.sh
 RUN chmod 755 /run.sh && chmod +x /run.sh

 ######################################
 # By default the app runs on port 8080

 EXPOSE 8080

 CMD ["sh", "/run.sh"]
