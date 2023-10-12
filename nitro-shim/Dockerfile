# Build proxies that facilitate communication with the enclave.
FROM golang:1.20 as builder

WORKDIR /src
COPY tools/ ./
RUN make -C ./viproxy/ viproxy
RUN make -C ./gvproxy/ gvproxy
RUN make -C ./eifbuild/ eifbuild

FROM amazonlinux:2.0.20230207.0

RUN echo "timeout=60.0" >> /etc/yum.conf

RUN amazon-linux-extras install aws-nitro-enclaves-cli -y && \
    yum install aws-nitro-enclaves-cli-devel wget curl strace awscli -y && \
    yum clean all && \
    rm -rf /var/cache/yum && \
    mkdir -p /enclave

RUN yum install build-essential cmake gcc -y
RUN yum install pkgconfig -y
RUN yum install openssl -y
RUN yum install libssl-dev openssl-devel -y

RUN curl https://sh.rustup.rs -sSf > rs.sh && sh rs.sh -y && \
    source "$HOME/.cargo/env" && \
    cargo install --git https://github.com/aws/aws-nitro-enclaves-image-format --example eif_build && \
    cp "$HOME/.cargo/bin/eif_build" /bin/eif_build

RUN ls $HOME/.cargo/bin

WORKDIR /enclave

COPY --from=builder /src/viproxy/viproxy /enclave/
COPY --from=builder /src/gvproxy/gvproxy /enclave/
COPY --from=builder /src/eifbuild/eifbuild /enclave/

COPY scripts/ /enclave/
