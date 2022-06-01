FROM phusion/baseimage:focal-1.0.0

# Use baseimage-docker's init system.
CMD ["/sbin/my_init"]

COPY build/cosfs/launcher/base/syslog-ng /etc/logrotate.d/syslog-ng

RUN apt update && apt upgrade -y
RUN apt update && apt install --no-install-recommends --no-install-suggests -y automake autotools-dev g++ git libcurl4-gnutls-dev libfuse-dev libssl-dev libxml2-dev make pkg-config fuse
RUN apt update && apt install --no-install-recommends --no-install-suggests -y libxml2 gdb strace \
    inetutils-tools iproute2 procps inetutils-ping net-tools telnet curl ca-certificates
RUN git clone https://github.com/tencentyun/cosfs /usr/cosfs && cd /usr/cosfs && ./autogen.sh && ./configure && make && make install
