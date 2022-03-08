FROM golang:1.16 as builder

WORKDIR /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud
ADD . .
RUN go build -v -a -ldflags '-extldflags "-static"' -o csi-tencentcloud-cfsturbo cmd/cfsturbo/main.go

FROM centos:7.6.1810

LABEL maintainers="TencentCloud TKE Authors"
LABEL description="TencentCloud CFS CSI Plugin"

COPY build/cfsturbo/lustre-client-2.12.4-1.el7.x86_64.rpm /tmp/lustre-client-2.12.4-1.el7.x86_64.rpm
COPY --from=builder /go/src/github.com/tencentcloud/kubernetes-csi-tencentcloud/csi-tencentcloud-cfsturbo /csi-tencentcloud-cfsturbo
RUN yum -y install nfs-utils && yum clean all && rpm -i --force --nodeps /tmp/lustre-client-2.12.4-1.el7.x86_64.rpm && rm -f /tmp/*
ENTRYPOINT ["/csi-tencentcloud-cfsturbo"]
