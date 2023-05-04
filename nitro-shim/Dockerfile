# Build proxies that facilitate communication with the enclave.
FROM golang:1.20 as builder

WORKDIR /src
COPY tools/ ./
RUN make -C ./viproxy/ viproxy
RUN make -C ./gvproxy/ gvproxy

FROM amazonlinux:2.0.20230207.0

RUN echo "timeout=60.0" >> /etc/yum.conf

RUN amazon-linux-extras install aws-nitro-enclaves-cli -y && \
    yum install aws-nitro-enclaves-cli-devel wget curl strace awscli -y && \
    yum clean all && \
    rm -rf /var/cache/yum && \
    mkdir -p /enclave

WORKDIR /enclave

COPY --from=builder /src/viproxy/viproxy /enclave/
COPY --from=builder /src/gvproxy/gvproxy /enclave/

COPY scripts/ /enclave/
