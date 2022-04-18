ARG GO_VERSION=1.18.1
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS builder
RUN mkdir /build
COPY *.go go.* /build/
WORKDIR /build

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o isg_exporter

FROM scratch AS target
COPY --from=builder /build/isg_exporter /
COPY modbus-mapping.yaml /
ENTRYPOINT [ "/isg_exporter" ]
