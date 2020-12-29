FROM kerberos/debian-opencv-ffmpeg:1.0.0 AS builder
MAINTAINER Kerberos.io

ARG gitlab_id
ARG gitlab_token

ENV GOROOT=/usr/local/go
ENV GOPATH=/go
ENV PATH=$GOPATH/bin:$GOROOT/bin:$PATH
ENV GOSUMDB=off

RUN mkdir -p /go/src/github.com/kerberos-io/opensource
COPY machinery /go/src/github.com/kerberos-io/opensource/machinery
COPY web /go/src/github.com/kerberos-io/opensource/web

# Build react
RUN apt-get install curl && curl -sL https://deb.nodesource.com/setup_14.x | bash - && \
    curl -sS https://dl.yarnpkg.com/debian/pubkey.gpg | apt-key add - && \
    echo "deb https://dl.yarnpkg.com/debian/ stable main" | tee /etc/apt/sources.list.d/yarn.list && \
    apt update && apt install yarn -y

RUN cd /go/src/github.com/kerberos-io/opensource/web && \
    npm install && yarn build

# Build golang
RUN cd /go/src/github.com/kerberos-io/opensource/backend && \
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

 # Let's create a /dist folder containing just the files necessary for runtime.
 # Later, it will be copied as the / (root) of the output image.
 WORKDIR /dist
 RUN cp -r /opensource ./

 # Optional: in case your application uses dynamic linking (often the case with CGO),
 # this will collect dependent libraries so they're later copied to the final image
 # NOTE: make sure you honor the license terms of the libraries you copy and distribute
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

 RUN apk add supervisor && mkdir -p /var/log/supervisor/
 ADD ./scripts/supervisor.conf /etc/supervisord.conf
 ADD ./scripts/run.sh /run.sh
 RUN chmod 755 /run.sh && chmod +x /run.sh

 EXPOSE 8080

 CMD ["sh", "/run.sh"]
