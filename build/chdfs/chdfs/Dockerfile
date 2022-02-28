FROM golang:1.14.1-stretch as builder

WORKDIR /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud
ADD . .
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -a -ldflags '-extldflags "-static"' -v -o csi-chdfs_${TARGETARCH}  cmd/chdfs/chdfs/main.go

FROM alpine:3.15.0

LABEL maintainers="TencentCloud TKE Authors"
LABEL description="TencentCloud CHDFS CSI Plugin"

ARG TARGETARCH
COPY --from=builder /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud/csi-chdfs_${TARGETARCH} /bin/csi-chdfs
COPY build/chdfs/chdfs/chdfs-csi.sh /chdfs-csi.sh
RUN chmod +x /chdfs-csi.sh
ENTRYPOINT ["/chdfs-csi.sh"]
