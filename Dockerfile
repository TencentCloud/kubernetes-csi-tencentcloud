FROM centos:7.5.1804
LABEL maintainers="TencentCloud TKE Authors"
LABEL description="TencentCloud CBS CSI Plugin"

RUN yum  install -y e2fsprogs xfsprogs && yum clean all

COPY _output/csi-tencentcloud-cbs /csi-tencentcloud-cbs
RUN chmod +x /csi-tencentcloud-cbs
CMD ["/csi-tencentcloud-cbs"]

