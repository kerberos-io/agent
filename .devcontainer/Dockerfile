FROM kerberos/base:4281f79 AS builder
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
ENV NODE_VERSION 16.17.0
RUN curl https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.1/install.sh | bash \
	&& . $NVM_DIR/nvm.sh \
	&& nvm install $NODE_VERSION \
	&& nvm alias default $NODE_VERSION \
	&& nvm use default

ENV NODE_PATH $NVM_DIR/v$NODE_VERSION/lib/node_modules
ENV PATH $NVM_DIR/versions/node/v$NODE_VERSION/bin:$PATH
RUN npm install -g yarn

##########################
# Dependencies for ansible

RUN pip install requests requests-oauthlib  kubernetes