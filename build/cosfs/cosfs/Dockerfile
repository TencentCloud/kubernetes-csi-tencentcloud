FROM golang:1.16 as builder

WORKDIR /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud
ADD . .
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -v -a -ldflags '-extldflags "-static"' -o csi-cos_${TARGETARCH} cmd/cosfs/cosfs/main.go

FROM alpine:3.15.0

LABEL maintainers="TencentCloud TKE Authors"
LABEL description="TencentCloud COS CSI Plugin"

ARG TARGETARCH
COPY --from=builder /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud/csi-cos_${TARGETARCH} /bin/csi-cos
RUN echo "hosts: files dns" > /etc/nsswitch.conf
ENTRYPOINT ["/bin/csi-cos"]
