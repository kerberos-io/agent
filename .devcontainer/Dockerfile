FROM mcr.microsoft.com/devcontainers/go:1.26-trixie

# Same pin as the main Dockerfile: Go 1.26.11 rejects moq-go-ffi v0.3.x's
# archives (LLVM bitcode), which breaks `go build -tags moq`.
ENV GOTOOLCHAIN=go1.26.10

# Install node environment
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    nodejs \
    npm \
    && rm -rf /var/lib/apt/lists/*

# Install ffmpeg
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    ffmpeg \
    libavcodec-extra \
    libavutil-dev \
    libavformat-dev \
    libavfilter-dev \
    libavdevice-dev \
    libswscale-dev \
    libswresample-dev \
    && rm -rf /var/lib/apt/lists/*

USER vscode

# Install go swagger
RUN go install github.com/swaggo/swag/cmd/swag@latest