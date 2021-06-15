ARG GO_VERSION=1.16.5
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION} AS builder
RUN mkdir /build
COPY *.go go.* /build/
WORKDIR /build

ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o isg_exporter

FROM scratch AS target
COPY --from=builder /build/isg_exporter /
ENTRYPOINT [ "/isg_exporter" ]
