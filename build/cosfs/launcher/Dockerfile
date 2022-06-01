FROM golang:1.16 as builder

WORKDIR /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud
ADD . .
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -v -a -ldflags '-extldflags "-static"' -o launcher_${TARGETARCH} cmd/cosfs/launcher/launcher.go

FROM ccr.ccs.tencentyun.com/tkeimages/csi-tencentcloud-cos:base.1

LABEL maintainers="TencentCloud TKE Authors"
LABEL description="TencentCloud COSFS CSI Plugin"

ARG TARGETARCH
COPY --from=builder /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud/launcher_${TARGETARCH} /bin/launcher
COPY build/cosfs/launcher/launcher.sh /etc/service/cos-launcher/run
RUN chmod +x /etc/service/cos-launcher/run
