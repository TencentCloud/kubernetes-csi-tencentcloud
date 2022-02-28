FROM golang:1.14.1-stretch as builder

WORKDIR /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud
ADD . .
ARG TARGETARCH
RUN CGO_ENABLED=0  GOOS=linux GOARCH=${TARGETARCH}  go build -v -o launcher_${TARGETARCH} -a -ldflags '-extldflags "-static"' cmd/chdfs/launcher/main.go

FROM ccr.ccs.tencentyun.com/ccs-dev/chdfs-fuse:v1.0.0

LABEL maintainers="TencentCloud TKE Authors"
LABEL description="TencentCloud CHDFS CSI Plugin"

ARG TARGETARCH
COPY --from=builder /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud/launcher_${TARGETARCH}  /bin/launcher
COPY build/chdfs/launcher/launcher.sh /bin/launcher_run
RUN chmod +x /bin/launcher_run && chmod +x /bin/launcher
ENTRYPOINT ["/bin/launcher_run"]
