FROM golang:1.16 as builder

WORKDIR /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud
ADD . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -v -a -ldflags '-extldflags "-static"' -o csi-tencentcloud-cfsturbo cmd/cfsturbo/main.go

FROM ccr.ccs.tencentyun.com/tkeimages/csi-tencentcloud-cfsturbo:base.1

LABEL maintainers="TencentCloud TKE Authors"
LABEL description="TencentCloud CFS CSI Plugin"

COPY --from=builder /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud/csi-tencentcloud-cfsturbo /csi-tencentcloud-cfsturbo
ENTRYPOINT ["/csi-tencentcloud-cfsturbo"]
