FROM golang:1.16 as builder

WORKDIR /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud
ADD . .
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -v -a -ldflags '-extldflags "-static"' -o csi-tencentcloud_${TARGETARCH} cmd/cbs/main.go

FROM ccr.ccs.tencentyun.com/tkeimages/csi-tencentcloud-cbs:base.2

LABEL maintainers="TencentCloud TKE Authors"
LABEL description="TencentCloud CBS CSI Plugin"

ARG TARGETARCH
COPY --from=builder /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud/csi-tencentcloud_${TARGETARCH} /csi-tencentcloud-cbs
CMD ["/csi-tencentcloud-cbs"]
