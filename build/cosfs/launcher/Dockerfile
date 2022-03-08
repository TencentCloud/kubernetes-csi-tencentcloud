FROM golang:1.15.15-stretch as builder

WORKDIR /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud
ADD . .
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -v -a -ldflags '-extldflags "-static"' -o launcher_${TARGETARCH} cmd/cosfs/launcher/launcher.go

FROM phusion/baseimage:focal-1.0.0
LABEL maintainers="TencentCloud TKE Authors"
LABEL description="TencentCloud COSFS CSI Plugin"
# Use baseimage-docker's init system.
CMD ["/sbin/my_init"]

COPY build/cosfs/launcher/syslog-ng /etc/logrotate.d/syslog-ng

RUN apt update && apt upgrade -y
RUN apt update && apt install --no-install-recommends --no-install-suggests -y automake autotools-dev g++ git libcurl4-gnutls-dev libfuse-dev libssl-dev libxml2-dev make pkg-config fuse
RUN apt update && apt install --no-install-recommends --no-install-suggests -y libxml2 gdb strace \
    inetutils-tools iproute2 procps inetutils-ping net-tools telnet curl ca-certificates
RUN git clone https://github.com/tencentyun/cosfs /usr/cosfs && cd /usr/cosfs && ./autogen.sh && ./configure && make && make install

ARG TARGETARCH
COPY --from=builder /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud/launcher_${TARGETARCH} /bin/launcher
COPY build/cosfs/launcher/launcher.sh /etc/service/cos-launcher/run
RUN chmod +x /etc/service/cos-launcher/run
