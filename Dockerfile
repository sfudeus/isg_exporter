FROM golang:1.11.2 AS builder
ENV GO111MODULE=on
RUN mkdir /build
COPY *.go go.* /build/
WORKDIR /build
RUN CGO_ENABLED=0 GOOS=linux go build -o isg_exporter

FROM scratch
COPY --from=builder /build/isg_exporter /
ENTRYPOINT [ "/isg_exporter" ]
