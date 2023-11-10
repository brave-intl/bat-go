# Build proxies that facilitate communication with the enclave.
FROM golang:1.20 as builder

WORKDIR /src
COPY tools/ ./
RUN make -C ./viproxy/ viproxy
RUN make -C ./gvproxy/ gvproxy
RUN make -C ./eifbuild/ eifbuild

FROM rust:1.67 as rust_builder
RUN cargo install --git https://github.com/aws/aws-nitro-enclaves-image-format --example eif_build

FROM amazonlinux:2.0.20230207.0

RUN echo "timeout=60.0" >> /etc/yum.conf

RUN amazon-linux-extras install aws-nitro-enclaves-cli -y && \
    yum install aws-nitro-enclaves-cli-devel awscli -y && \
    yum clean all && \
    rm -rf /var/cache/yum && \
    mkdir -p /enclave

RUN yum install openssl openssl-devel -y

WORKDIR /enclave

COPY --from=builder /src/viproxy/viproxy /enclave/
COPY --from=builder /src/gvproxy/gvproxy /enclave/
COPY --from=builder /src/eifbuild/eifbuild /enclave/
COPY --from=rust_builder /usr/local/cargo/bin/eif_build /bin/eif_build
COPY --from=rust_builder /usr/lib/x86_64-linux-gnu/libssl.so.1.1 /usr/lib64/libssl.so.1.1


COPY scripts/ /enclave/
