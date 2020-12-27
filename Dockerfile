FROM debian:buster AS builder
MAINTAINER Kerberos.io

#################################
# Surpress Upstart errors/warning

RUN dpkg-divert --local --rename --add /sbin/initctl
RUN ln -sf /bin/true /sbin/initctl

#############################################
# Let the container know that there is no tty

ENV DEBIAN_FRONTEND noninteractive

#################################
# Clone and build FFMpeg & OpenCV

RUN apt-get update && apt-get upgrade -y && apt-get -y --no-install-recommends install git cmake wget dh-autoreconf autotools-dev autoconf automake gcc build-essential libtool make ca-certificates supervisor nasm zlib1g-dev tar libx264. unzip wget pkg-config libavresample-dev && \
	git clone https://github.com/FFmpeg/FFmpeg && \
	cd FFmpeg && git checkout remotes/origin/release/4.0 && \
	./configure --prefix=/usr/local --target-os=linux --enable-nonfree --enable-libx264 --enable-gpl --enable-shared && \
	make -j8 && \
  make install && \
  cd .. && rm -rf FFmpeg
RUN	wget -O opencv.zip https://github.com/opencv/opencv/archive/4.0.0.zip && \
	unzip opencv.zip && mv opencv-4.0.0 opencv && cd opencv && mkdir build && cd build && \
	cmake -D CMAKE_BUILD_TYPE=RELEASE \
		 -D CMAKE_INSTALL_PREFIX=/usr/ \
		 -D OPENCV_GENERATE_PKGCONFIG=YES \
		 -D BUILD_TESTS=OFF \
		 -D OPENCV_ENABLE_NONFREE=ON \
		 #-D BUILD_opencv_dnn=OFF \
		 -D BUILD_opencv_ml=OFF \
		 -D BUILD_opencv_stitching=OFF \
		 -D BUILD_opencv_ts=OFF \
		 -D BUILD_opencv_java_bindings_generator=OFF \
		 -D BUILD_opencv_python_bindings_generator=OFF \
		 -D INSTALL_PYTHON_EXAMPLES=OFF \
		 -D BUILD_EXAMPLES=OFF .. && make -j8 && make install && cd ../.. && rm -rf opencv*

############################
# Build Golang

ENV GOROOT=/usr/local/go
ENV GOPATH=/go
ENV PATH=$GOPATH/bin:$GOROOT/bin:$PATH

RUN apt-get install -y git
RUN ARCH=$([ "$(uname -m)" = "armv7l" ] && echo "armv6l" || echo "amd64") && wget "https://dl.google.com/go/go1.14.2.linux-$ARCH.tar.gz" && \
	tar -xvf "go1.14.2.linux-$ARCH.tar.gz" && \
	mv go /usr/local

RUN mkdir -p /go/src/github.com/kerberos-io/opensource
COPY backend /go/src/github.com/kerberos-io/opensource/backend
RUN cd /go/src/github.com/kerberos-io/opensource/backend && \
   go mod download && \
   go build -o main && \
	 mkdir -p /opensource && \
	 mv main /opensource && \
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
 RUN mkdir ./usr/lib
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
