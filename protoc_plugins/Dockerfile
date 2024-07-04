FROM --platform=${TARGETPLATFORM:-linux/amd64} golang:1.22-alpine as builder

ENV CGO_ENABLED=0

WORKDIR /src
COPY protoc_plugins/ .
RUN go mod download
RUN go mod tidy

RUN go build -trimpath -ldflags "-s" -o protoc-gen-php-grpc-plugin protoc-gen-php-grpc/main.go

FROM scratch

ARG APP_VERSION=""

# Runtime dependencies
LABEL org.opencontainers.image.title="protoc-gen-php-grpc"
LABEL org.opencontainers.image.description="protoc plugin for generating PHP gRPC service stubs"
LABEL org.opencontainers.image.url="https://roadrunner.dev"
LABEL org.opencontainers.image.source="https://github.com/roadrunner-server/grpc"
LABEL org.opencontainers.image.vendor="SpiralScout"
LABEL org.opencontainers.image.version="$APP_VERSION"
LABEL org.opencontainers.image.created="$BUILD_TIME"
LABEL org.opencontainers.image.licenses="MIT"

COPY --from=builder /src/protoc-gen-php-grpc-plugin /

ENTRYPOINT ["/protoc-gen-php-grpc-plugin"]
