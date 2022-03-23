FROM amazonlinux:2.0.20210126.0

RUN amazon-linux-extras install aws-nitro-enclaves-cli -y && \
    yum install aws-nitro-enclaves-cli-devel wget strace -y && \
    yum clean all && \
    rm -rf /var/cache/yum && \
    mkdir -p /enclave

WORKDIR /enclave

COPY scripts/ /enclave/
