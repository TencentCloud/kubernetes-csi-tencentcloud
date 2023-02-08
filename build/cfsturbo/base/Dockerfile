FROM centos:7.6.1810

COPY build/cfsturbo/base/lustre-client-2.12.4-1.el7.x86_64.rpm /tmp/lustre-client-2.12.4-1.el7.x86_64.rpm
RUN yum -y install nfs-utils && yum clean all && rpm -i --force --nodeps /tmp/lustre-client-2.12.4-1.el7.x86_64.rpm && rm -f /tmp/*
