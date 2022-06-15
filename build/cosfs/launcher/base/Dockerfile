FROM phusion/baseimage:focal-1.0.0

# Use baseimage-docker's init system.
CMD ["/sbin/my_init"]

COPY build/cosfs/launcher/base/syslog-ng /etc/logrotate.d/syslog-ng

RUN apt update && apt upgrade -y &&\
    apt install --no-install-recommends --no-install-suggests -y automake autotools-dev g++ git libcurl4-gnutls-dev libfuse-dev libssl-dev libxml2-dev make pkg-config fuse &&\
    apt install --no-install-recommends --no-install-suggests -y libxml2 gdb strace inetutils-tools iproute2 procps inetutils-ping net-tools telnet curl ca-certificates &&\
    git clone https://github.com/tencentyun/cosfs /usr/cosfs && cd /usr/cosfs && ./autogen.sh && ./configure && make && make install &&\
    apt install --no-install-recommends --no-install-suggests -y fuse openjdk-11-jdk &&\
    curl -LO https://cos-data-lake-release-1253960454.cos.ap-guangzhou.myqcloud.com/goosefs-lite/goosefs-lite-1.0.0.tar.gz &&\
    tar zxvf goosefs-lite-1.0.0.tar.gz -C / && mv /goosefs-lite-1.0.0 /goosefs-lite && rm -f goosefs-lite-1.0.0.tar.gz

ADD build/cosfs/launcher/base/goosefs-lite /goosefs-lite/bin/

ARG TARGETARCH
ADD java-11-konajdk_11.0.15-2_amd64.deb /
RUN if [ "$TARGETARCH" = "amd64" ]; then dpkg -i /java-11-konajdk_11.0.15-2_amd64.deb && ln -s /usr/lib/jvm/TencentKona-11.0.15.b1/bin/java /etc/alternatives/java && \
    ln -s /etc/alternatives/java /usr/bin/java; fi
