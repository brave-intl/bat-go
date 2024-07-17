FROM debian:bookworm

ARG DEBIAN_FRONTEND=noninteractive
ARG GOLANG_VERSION=1.22.4

RUN apt-get update \
    && apt-get install -y -qq \
        tmux curl man less openssh-client openssl util-linux \
        python3 ipython3 python-is-python3 \
        socat lsof wget diffutils \
        git make \
        redis-tools \
        node-sshpk

# Install Go
RUN set -x && curl -L -o /var/tmp/go.tgz \
    "https://go.dev/dl/go$GOLANG_VERSION.linux-amd64.tar.gz" \
    && tar -C /usr/local -xf /var/tmp/go.tgz \
    && rm /var/tmp/go.tgz \
    && find /usr/local/go/bin -type f -perm /001 \
        -exec ln -s -t /usr/local/bin '{}' +

# Use arbitrary id, not 1000, to always test the code to sync the user id.
RUN useradd -m -u 12345 user
