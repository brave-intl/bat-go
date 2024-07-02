FROM debian:bookworm AS base

ARG DEBIAN_FRONTEND=noninteractive
ARG GOLANG_VERSION=1.22.4

RUN apt-get update \
    && apt-get install -y -qq \
        tmux curl man less \
        python3 git make

# Install Go
RUN set -x && curl -L -o /var/tmp/go.tgz \
    "https://go.dev/dl/go$GOLANG_VERSION.linux-amd64.tar.gz" \
    && tar -C /usr/local -xf /var/tmp/go.tgz \
    && rm /var/tmp/go.tgz \
    && find /usr/local/go/bin -type f -perm /001 \
        -exec ln -s -t /usr/local/bin '{}' +

RUN useradd -m user

RUN mkdir /build && chown user:user /build

USER user
WORKDIR /home/user

#CMD [ "sleep", "infinity" ]

# A helper stage to hold Go sources with go.mod and related infrequently changed
# files moved to separated directory so they can be copied later before the *.go
# files to allow to cache downloaded Go modules in a Docker layer independent
# from more frequently changed sources.
FROM base as sources

COPY --chown=user:user . /build/repo
RUN mkdir /build/mod-files && cd /build/repo && rm -rf .git \
    && find . -name go.\* | xargs tar cf - | tar -C /build/mod-files -xf - \
    && find . -name go.\* -delete

FROM base as image

RUN mkdir -p .cache

COPY --link --from=sources --chown=user:user /build/mod-files/ /build/src/

RUN cd /build/src/main && go mod download -x

COPY --link --from=sources --chown=user:user /build/repo/ /build/src/

RUN cd /build/src/main \
    && CGO_ENABLED=0 GOOS=linux go build \
        -o /build/bat-go main.go

