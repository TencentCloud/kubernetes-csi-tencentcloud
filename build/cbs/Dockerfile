FROM golang:1.14 as builder

WORKDIR /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud
ADD . .
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -v -a -ldflags '-extldflags "-static"' -o csi-tencentcloud_${TARGETARCH} cmd/cbs/main.go

FROM alpine:3.15.0

LABEL maintainers="TencentCloud TKE Authors"
LABEL description="TencentCloud CBS CSI Plugin"

ARG TARGETARCH
COPY --from=builder /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud/csi-tencentcloud_${TARGETARCH} /csi-tencentcloud-cbs
RUN apk add e2fsprogs xfsprogs findmnt blkid e2fsprogs-extra && echo "hosts: files dns" > /etc/nsswitch.conf
CMD ["/csi-tencentcloud-cbs"]
