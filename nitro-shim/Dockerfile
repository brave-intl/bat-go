FROM nixos/nix:2.21.4 as kernel-builder

WORKDIR /build

RUN git clone https://github.com/brave/nitro-enclave-kernel

WORKDIR /build/nitro-enclave-kernel

RUN nix-build

# Build proxies that facilitate communication with the enclave.
FROM golang:1.20 as go-builder

WORKDIR /src
COPY tools/ ./
RUN make -C ./viproxy/ viproxy
RUN make -C ./gvproxy/ gvproxy
RUN make -C ./eifbuild/ eifbuild

FROM rust:1 as rust-builder

RUN cargo install vsock-relay

FROM amazonlinux:2023

RUN echo "timeout=60.0" >> /etc/yum.conf

RUN yum update && \
    yum install aws-nitro-enclaves-cli aws-nitro-enclaves-cli-devel wget strace awscli gcc -y && \
    yum clean all && \
    rm -rf /var/cache/yum && \
    mkdir -p /enclave

WORKDIR /enclave

COPY --from=go-builder /src/viproxy/viproxy /enclave/
COPY --from=go-builder /src/gvproxy/gvproxy /enclave/
COPY --from=go-builder /src/eifbuild/eifbuild /enclave/
COPY --from=rust-builder /usr/local/cargo/bin/vsock-relay /enclave/
COPY --from=kernel-builder /build/nitro-enclave-kernel/result/bzImage /usr/share/nitro_enclaves/blobs
COPY --from=kernel-builder /build/nitro-enclave-kernel/bzImage.config /usr/share/nitro_enclaves/blobs
COPY --from=kernel-builder /build/nitro-enclave-kernel/result/nsm.ko /usr/share/nitro_enclaves/blobs

COPY scripts/ /enclave/
