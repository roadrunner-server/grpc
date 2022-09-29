FROM golang as builder

ARG APP_VERSION=""
ENV GOOS=linux GOARCH=amd64 CGO_ENABLED=0

WORKDIR /src
COPY protoc_plugins/ .
RUN go mod download
RUN go mod tidy -go=1.19

RUN go build -trimpath -ldflags "-s" -o protoc-gen-php-grpc protoc-gen-php-grpc/main.go

FROM scratch

# Runtime dependencies
LABEL "build.buf.plugins.runtime_library_versions.0.name"="github.com/roadrunner-server/grpc/v2/protoc_plugins"
LABEL "build.buf.plugins.runtime_library_versions.0.version"="$APP_VERSION"
LABEL "build.buf.plugins.runtime_library_versions.1.name"="google.golang.org/protobuf"
LABEL "build.buf.plugins.runtime_library_versions.1.version"="v1.27.1"

COPY --from=builder /src/protoc-gen-php-grpc /

ENTRYPOINT ["/protoc-gen-php-grpc"]
