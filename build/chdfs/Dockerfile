FROM golang:1.14.1-stretch as builder

WORKDIR /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud
ADD . .
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -v -a -ldflags '-extldflags "-static"' -o csi-chdfs_${TARGETARCH} cmd/chdfs/main.go

FROM ccr.ccs.tencentyun.com/ccs-dev/chdfs-fuse:v1.0.0

LABEL maintainers="TencentCloud TKE Authors"
LABEL description="TencentCloud CHDFS CSI Plugin"

ARG TARGETARCH
COPY --from=builder /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud/csi-chdfs_${TARGETARCH} /bin/csi-chdfs
ENTRYPOINT ["/bin/csi-chdfs"]
