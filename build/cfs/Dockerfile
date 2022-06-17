FROM golang:1.16 as builder

WORKDIR /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud
ADD . .
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -v -a -ldflags '-extldflags "-static"' -o csi-tencentcloud-cfs_${TARGETARCH} cmd/cfs/main.go

FROM ccr.ccs.tencentyun.com/tkeimages/csi-tencentcloud-cfs:base.1

LABEL maintainers="TencentCloud TKE Authors"
LABEL description="TencentCloud CFS CSI Plugin"

ARG TARGETARCH
COPY --from=builder /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud/csi-tencentcloud-cfs_${TARGETARCH} /csi-tencentcloud-cfs
ENTRYPOINT ["/csi-tencentcloud-cfs"]
