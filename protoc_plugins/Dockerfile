FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

ENV CGO_ENABLED=0

WORKDIR /src
COPY protoc_plugins/go.mod protoc_plugins/go.sum ./
RUN go mod download

COPY protoc_plugins/ .

RUN set -eux; \
	goarm=""; \
	goamd64=""; \
	if [ "${TARGETARCH:-}" = "arm" ] && [ -n "${TARGETVARIANT:-}" ]; then goarm="${TARGETVARIANT#v}"; fi; \
	if [ "${TARGETARCH:-}" = "amd64" ] && [ -n "${TARGETVARIANT:-}" ]; then goamd64="${TARGETVARIANT#v}"; fi; \
	GOOS="${TARGETOS:-linux}" GOARCH="${TARGETARCH:-amd64}" GOARM="${goarm}" GOAMD64="${goamd64}" \
	go build -trimpath -ldflags "-s -w" -o /out/protoc-gen-php-grpc-plugin ./protoc-gen-php-grpc

FROM scratch

ARG APP_VERSION=""
ARG BUILD_TIME=""

# Runtime dependencies
LABEL org.opencontainers.image.title="protoc-gen-php-grpc"
LABEL org.opencontainers.image.description="protoc plugin for generating PHP gRPC service stubs"
LABEL org.opencontainers.image.url="https://roadrunner.dev"
LABEL org.opencontainers.image.source="https://github.com/roadrunner-server/grpc"
LABEL org.opencontainers.image.vendor="SpiralScout"
LABEL org.opencontainers.image.version="${APP_VERSION}"
LABEL org.opencontainers.image.created="${BUILD_TIME}"
LABEL org.opencontainers.image.licenses="MIT"

COPY --from=builder /out/protoc-gen-php-grpc-plugin /protoc-gen-php-grpc-plugin

ENTRYPOINT ["/protoc-gen-php-grpc-plugin"]
