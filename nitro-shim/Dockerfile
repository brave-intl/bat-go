# Build proxies that facilitate communication with the enclave.
FROM golang:1.20 as builder

WORKDIR /src
COPY tools/ ./
RUN make -C ./viproxy/ viproxy
RUN make -C ./gvproxy/ gvproxy
RUN make -C ./eifbuild/ eifbuild

FROM rust:1.67 as rust_builder
RUN apt update && apt install -y libssl-dev musl-tools musl-dev
ENV OPENSSL_STATIC=1
RUN rustup target add x86_64-unknown-linux-musl
RUN ln -s /usr/include/x86_64-linux-gnu/openssl/opensslconf.h /usr/include/openssl/
RUN git clone https://github.com/aws/aws-nitro-enclaves-image-format 
RUN cd aws-nitro-enclaves-image-format && \
    sed -i -e 's/^openssl.*/openssl = { version = "0.10", features = ["vendored"] }/g' Cargo.toml && \
    cargo install --target x86_64-unknown-linux-musl --path . --example eif_build

FROM amazonlinux:2.0.20230207.0

RUN echo "timeout=60.0" >> /etc/yum.conf

RUN amazon-linux-extras install aws-nitro-enclaves-cli -y && \
    yum install aws-nitro-enclaves-cli-devel awscli -y && \
    yum clean all && \
    rm -rf /var/cache/yum && \
    mkdir -p /enclave

WORKDIR /enclave

COPY --from=builder /src/viproxy/viproxy /enclave/
COPY --from=builder /src/gvproxy/gvproxy /enclave/
COPY --from=builder /src/eifbuild/eifbuild /enclave/
COPY --from=rust_builder /usr/local/cargo/bin/eif_build /bin/eif_build

COPY scripts/ /enclave/
