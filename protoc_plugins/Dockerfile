FROM golang:1.26-alpine AS builder

ARG TARGETPLATFORM

ENV CGO_ENABLED=0

WORKDIR /src
# https://buf.build/docs/bsr/remote-plugins/custom-plugins/#build-a-docker-image
RUN test "${TARGETPLATFORM}" = "linux/amd64" || (echo "buf plugin image must be built for linux/amd64" && exit 1)

COPY protoc_plugins/go.mod protoc_plugins/go.sum ./
RUN go mod download

COPY protoc_plugins/ .

RUN go build -trimpath -ldflags "-s -w" -o /out/protoc-gen-php-grpc-plugin ./protoc-gen-php-grpc

FROM scratch

# Supply real metadata at build time, for example:
# docker build --platform linux/amd64 --build-arg BUILD_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)" --build-arg APP_VERSION="vX.Y.Z" -f protoc_plugins/Dockerfile .
ARG APP_VERSION="unknown"
ARG BUILD_TIME="unknown"

# Runtime dependencies
LABEL org.opencontainers.image.title="protoc-gen-php-grpc"
LABEL org.opencontainers.image.description="protoc plugin for generating PHP gRPC service stubs"
LABEL org.opencontainers.image.url="https://roadrunner.dev"
LABEL org.opencontainers.image.source="https://github.com/roadrunner-server/grpc"
LABEL org.opencontainers.image.vendor="SpiralScout"
LABEL org.opencontainers.image.version="${APP_VERSION}"
LABEL org.opencontainers.image.created="${BUILD_TIME}"
LABEL org.opencontainers.image.licenses="MIT"

COPY --from=builder --chown=65532:65532 /out/protoc-gen-php-grpc-plugin /protoc-gen-php-grpc-plugin

USER 65532:65532

ENTRYPOINT ["/protoc-gen-php-grpc-plugin"]
